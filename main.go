package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Aayush9029/funnelr/internal/clipboard"
	"github.com/Aayush9029/funnelr/internal/ports"
	"github.com/Aayush9029/funnelr/internal/proxylog"
	"github.com/Aayush9029/funnelr/internal/state"
	"github.com/Aayush9029/funnelr/internal/tailscale"
	"github.com/Aayush9029/funnelr/internal/tui"
	"github.com/Aayush9029/funnelr/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		ui.Error("%s", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return runInteractive()
	}
	switch args[0] {
	case "daemon":
		return runDaemon(args[1:])
	case "stop", "off":
		return runStop()
	case "status":
		return runStatus()
	case "logs", "log":
		return runLogs(args[1:])
	case "help", "-h", "--help":
		showHelp()
		return nil
	case "version", "-v", "--version":
		fmt.Printf("funnelr %s\n", version)
		return nil
	default:
		port, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("unknown command: %s", args[0])
		}
		return expose(port)
	}
}

func runInteractive() error {
	ctx := context.Background()
	if _, err := tailscale.New().Check(ctx); err != nil {
		return err
	}
	open := ports.OpenOnly(ports.Defaults, 120*time.Millisecond)
	var active *state.Session
	if s, err := state.Load(); err == nil {
		active = &s
	}
	model := tui.NewModel(open, active)
	finalModel, err := tea.NewProgram(model).Run()
	if err != nil {
		return err
	}
	result := finalModel.(tui.Model).Result()
	switch result.Action {
	case tui.ActionExpose:
		return expose(result.Port)
	case tui.ActionStop:
		return runStop()
	case tui.ActionLogs:
		return tailLog(state.LogPath(result.Port))
	default:
		return nil
	}
}

func expose(port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}
	if !ports.IsOpen(port, 300*time.Millisecond) {
		return fmt.Errorf("localhost:%d is not accepting TCP connections", port)
	}

	ctx := context.Background()
	ts := tailscale.New()
	status, err := ts.Check(ctx)
	if err != nil {
		return err
	}

	_ = stopActive(false)

	proxyPort, err := proxylog.FreePort()
	if err != nil {
		return fmt.Errorf("finding proxy port: %w", err)
	}
	logPath := state.LogPath(port)
	if err := startDaemon(port, proxyPort, logPath); err != nil {
		return err
	}
	time.Sleep(250 * time.Millisecond)
	if !waitForPort(proxyPort, 2*time.Second) {
		return fmt.Errorf("proxy failed to start on localhost:%d", proxyPort)
	}

	if err := ts.StartFunnel(ctx, proxyPort); err != nil {
		_ = stopPID(lastDaemonPID)
		return err
	}
	url, err := ts.FunnelURL(ctx, status.Self.DNSName)
	if err != nil {
		_ = stopPID(lastDaemonPID)
		return err
	}

	s := state.Session{
		TargetPort: port,
		ProxyPort:  proxyPort,
		PID:        daemonPID(),
		URL:        url,
		LogPath:    logPath,
		StartedAt:  time.Now(),
	}
	if err := state.Save(s); err != nil {
		return err
	}

	fmt.Println(url)
	if clipboard.Copy(url) {
		ui.Status("copied to clipboard")
	}
	ui.Success("localhost:%d is public at %s", port, url)
	ui.Status("logs: %s", logPath)
	return nil
}

func startDaemon(targetPort, proxyPort int, logPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "daemon", "--target", strconv.Itoa(targetPort), "--proxy", strconv.Itoa(proxyPort), "--log", logPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting proxy daemon: %w", err)
	}
	lastDaemonPID = cmd.Process.Pid
	return cmd.Process.Release()
}

var lastDaemonPID int

func daemonPID() int {
	return lastDaemonPID
}

func runDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	target := fs.Int("target", 0, "target local port")
	proxy := fs.Int("proxy", 0, "proxy local port")
	logPath := fs.String("log", "", "log path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target <= 0 || *proxy <= 0 || *logPath == "" {
		return errors.New("daemon requires --target, --proxy, and --log")
	}
	ctx := signalContext()
	return proxylog.Server{TargetPort: *target, ProxyPort: *proxy, LogPath: *logPath}.Serve(ctx)
}

func runStop() error {
	return stopActive(true)
}

func stopActive(verbose bool) error {
	if s, err := state.Load(); err == nil && s.PID > 0 {
		_ = stopPID(s.PID)
	}
	if err := tailscale.New().StopFunnel(context.Background()); err != nil {
		if !strings.Contains(err.Error(), "no funnel") && !strings.Contains(err.Error(), "not running") {
			return err
		}
	}
	if err := state.Clear(); err != nil {
		return err
	}
	if verbose {
		ui.Success("stopped funnelr")
	}
	return nil
}

func stopDaemon() error {
	s, err := state.Load()
	if err != nil || s.PID <= 0 {
		return nil
	}
	return stopPID(s.PID)
}

func stopPID(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	_ = process.Signal(syscall.SIGTERM)
	return nil
}

func runStatus() error {
	s, err := state.Load()
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("no active funnelr session")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Printf("url: %s\n", s.URL)
	fmt.Printf("target: localhost:%d\n", s.TargetPort)
	fmt.Printf("proxy: localhost:%d\n", s.ProxyPort)
	fmt.Printf("pid: %d\n", s.PID)
	fmt.Printf("logs: %s\n", s.LogPath)
	return nil
}

func runLogs(args []string) error {
	path := ""
	if len(args) > 0 {
		port, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid port: %s", args[0])
		}
		path = state.LogPath(port)
	} else if s, err := state.Load(); err == nil {
		path = s.LogPath
	}
	if path == "" {
		return errors.New("no log target; pass a port or start a session")
	}
	return tailLog(path)
}

func tailLog(path string) error {
	cmd := exec.Command("tail", "-f", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func waitForPort(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 80*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func signalContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signalNotify(ch, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-ch
		cancel()
	}()
	return ctx
}

var signalNotify = func(ch chan<- os.Signal, sig ...os.Signal) {
	signal.Notify(ch, sig...)
}

func showHelp() {
	ui.Header("funnelr")
	fmt.Println("Usage:")
	fmt.Println("  funnelr             scan common ports and pick one")
	fmt.Println("  funnelr <port>      expose localhost:<port>")
	fmt.Println("  funnelr status      show active session")
	fmt.Println("  funnelr logs [port] tail request logs")
	fmt.Println("  funnelr stop        stop active tunnel")
	fmt.Println("  funnelr --version   show version")
}

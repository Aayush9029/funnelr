package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Aayush9029/funnelr/internal/funnel"
	"github.com/Aayush9029/funnelr/internal/ports"
	"github.com/Aayush9029/funnelr/internal/proxylog"
	"github.com/Aayush9029/funnelr/internal/state"
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
	service := funnel.New()
	if err := service.Check(ctx); err != nil {
		return err
	}
	open := ports.OpenOnly(ports.Defaults, 120*time.Millisecond)
	active, err := funnel.LoadSession()
	if err != nil {
		return err
	}
	model := tui.NewModel(open, active, service.Expose, service.Stop)
	_, err = tea.NewProgram(model).Run()
	return err
}

func expose(port int) error {
	result, err := funnel.New().Expose(context.Background(), port, ui.Status)
	if err != nil {
		return err
	}

	fmt.Printf("\n%s\n\n", result.Session.URL)
	if result.Copied {
		ui.Status("copied to clipboard")
	}
	ui.Success("localhost:%d is public at %s", result.Session.TargetPort, result.Session.URL)
	ui.Status("logs: %s", result.Session.LogPath)
	return nil
}

func runDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	target := fs.Int("target", 0, "target local port")
	proxy := fs.Int("proxy", 0, "proxy local port")
	logPath := fs.String("log", "", "log path")
	statsPath := fs.String("stats", "", "stats path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target <= 0 || *proxy <= 0 || *logPath == "" {
		return errors.New("daemon requires --target, --proxy, and --log")
	}
	ctx := signalContext()
	return proxylog.Server{TargetPort: *target, ProxyPort: *proxy, LogPath: *logPath, StatsPath: *statsPath}.Serve(ctx)
}

func runStop() error {
	if err := funnel.New().Stop(context.Background()); err != nil {
		return err
	}
	ui.Success("stopped funnelr")
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
	label := ""
	if len(args) > 0 {
		port, err := strconv.Atoi(args[0])
		if err != nil || port <= 0 || port > 65535 {
			return fmt.Errorf("invalid port %q; use a number between 1 and 65535", args[0])
		}
		if s, err := state.Load(); err == nil && s.TargetPort == port && s.LogPath != "" {
			path = s.LogPath
		} else {
			path = state.LatestLogPath(port)
		}
		label = fmt.Sprintf("localhost:%d", port)
	} else if s, err := state.Load(); err == nil {
		path = s.LogPath
		label = fmt.Sprintf("active tunnel localhost:%d", s.TargetPort)
	}
	if path == "" {
		return errors.New("no active tunnel logs; run 'funnelr' to expose a port, or pass a port with existing logs")
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no logs found for %s", label)
		}
		return fmt.Errorf("opening logs for %s: %w", label, err)
	}
	ui.Status("tailing logs for %s", label)
	return tailLog(path)
}

func tailLog(path string) error {
	cmd := exec.Command("tail", "-f", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
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

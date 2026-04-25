package funnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Aayush9029/funnelr/internal/clipboard"
	"github.com/Aayush9029/funnelr/internal/ports"
	"github.com/Aayush9029/funnelr/internal/proxylog"
	"github.com/Aayush9029/funnelr/internal/state"
	"github.com/Aayush9029/funnelr/internal/tailscale"
)

type ProgressFunc func(string, ...any)

type Service struct {
	Tailscale tailscale.Client
}

type ExposeResult struct {
	Session state.Session
	Copied  bool
}

func New() Service {
	return Service{Tailscale: tailscale.New()}
}

func (s Service) Check(ctx context.Context) error {
	_, err := s.Tailscale.Check(ctx)
	return err
}

func (s Service) Expose(ctx context.Context, port int, progress ProgressFunc) (ExposeResult, error) {
	if port <= 0 || port > 65535 {
		return ExposeResult{}, fmt.Errorf("invalid port: %d", port)
	}
	report(progress, "checking localhost:%d", port)
	if !ports.IsOpen(port, 300*time.Millisecond) {
		return ExposeResult{}, fmt.Errorf("localhost:%d is not accepting TCP connections", port)
	}

	report(progress, "checking tailscale")
	status, err := s.Tailscale.Check(ctx)
	if err != nil {
		return ExposeResult{}, err
	}

	_ = s.Stop(ctx)

	proxyPort, err := proxylog.FreePort()
	if err != nil {
		return ExposeResult{}, fmt.Errorf("finding proxy port: %w", err)
	}

	logPath := state.LogPath(port)
	statsPath := state.StatsPath(port)
	report(progress, "starting request logger on localhost:%d", proxyPort)
	pid, err := startDaemon(port, proxyPort, logPath, statsPath)
	if err != nil {
		return ExposeResult{}, err
	}
	if !waitForPort(proxyPort, 2*time.Second) {
		_ = stopPID(pid)
		return ExposeResult{}, fmt.Errorf("proxy failed to start on localhost:%d", proxyPort)
	}

	report(progress, "starting tailscale funnel")
	if err := s.Tailscale.StartFunnel(ctx, proxyPort); err != nil {
		_ = stopPID(pid)
		return ExposeResult{}, err
	}

	report(progress, "resolving public url")
	url, err := s.Tailscale.FunnelURL(ctx, status.Self.DNSName)
	if err != nil {
		_ = stopPID(pid)
		return ExposeResult{}, err
	}

	session := state.Session{
		TargetPort: port,
		ProxyPort:  proxyPort,
		PID:        pid,
		URL:        url,
		LogPath:    logPath,
		StatsPath:  statsPath,
		StartedAt:  time.Now(),
	}
	if err := state.Save(session); err != nil {
		return ExposeResult{}, err
	}

	return ExposeResult{Session: session, Copied: clipboard.Copy(url)}, nil
}

func (s Service) Stop(ctx context.Context) error {
	if session, err := state.Load(); err == nil && session.PID > 0 {
		_ = stopPID(session.PID)
	}
	if err := s.Tailscale.StopFunnel(ctx); err != nil {
		if !strings.Contains(err.Error(), "no funnel") && !strings.Contains(err.Error(), "not running") {
			return err
		}
	}
	return state.Clear()
}

func LoadSession() (*state.Session, error) {
	session, err := state.Load()
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func startDaemon(targetPort, proxyPort int, logPath, statsPath string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	daemonLog, err := os.OpenFile(state.DaemonLogPath(targetPort), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer daemonLog.Close()
	cmd := exec.Command(exe, "daemon",
		"--target", strconv.Itoa(targetPort),
		"--proxy", strconv.Itoa(proxyPort),
		"--log", logPath,
		"--stats", statsPath,
	)
	cmd.Stdout = daemonLog
	cmd.Stderr = daemonLog
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting proxy daemon: %w", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		return 0, err
	}
	return pid, nil
}

func stopPID(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	_ = process.Signal(syscall.SIGTERM)
	return nil
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

func report(progress ProgressFunc, format string, args ...any) {
	if progress != nil {
		progress(format, args...)
	}
}

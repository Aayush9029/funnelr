package tailscale

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

type CLI struct{}

func (CLI) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "tailscale", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return out, fmt.Errorf("%w: %s", err, msg)
		}
		return out, err
	}
	return out, nil
}

type Client struct {
	Runner Runner
}

type Status struct {
	Version      string `json:"Version"`
	BackendState string `json:"BackendState"`
	Self         struct {
		DNSName      string   `json:"DNSName"`
		Capabilities []string `json:"Capabilities"`
	} `json:"Self"`
}

func New() Client {
	return Client{Runner: CLI{}}
}

func (c Client) Check(ctx context.Context) (Status, error) {
	if _, err := exec.LookPath("tailscale"); err != nil {
		return Status{}, errors.New("tailscale CLI not found in PATH")
	}

	out, err := c.Runner.Run(ctx, "status", "--json")
	if err != nil {
		return Status{}, fmt.Errorf("tailscale status failed: %w", err)
	}
	var st Status
	if err := json.Unmarshal(out, &st); err != nil {
		return Status{}, fmt.Errorf("parsing tailscale status: %w", err)
	}
	if st.BackendState != "Running" {
		return st, fmt.Errorf("tailscale is not up (state: %s)", st.BackendState)
	}
	if strings.TrimSpace(st.Self.DNSName) == "" {
		return st, errors.New("tailscale DNS name missing; enable MagicDNS/HTTPS certificates")
	}
	if !st.HasFunnel() {
		return st, errors.New("tailscale Funnel is not enabled for this machine")
	}
	if !versionAtLeast(st.Version, 1, 52) {
		return st, fmt.Errorf("tailscale %s is too old; install 1.52 or newer", st.Version)
	}
	return st, nil
}

func (s Status) HasFunnel() bool {
	for _, cap := range s.Self.Capabilities {
		if cap == "funnel" || strings.Contains(strings.ToLower(cap), "/cap/funnel") {
			return true
		}
	}
	return false
}

func (c Client) StartFunnel(ctx context.Context, proxyPort int) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := c.Runner.Run(ctx, "funnel", "--bg", "--yes", "localhost:"+strconv.Itoa(proxyPort))
	if err != nil {
		return fmt.Errorf("starting tailscale funnel: %w", err)
	}
	return nil
}

func (c Client) StopFunnel(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	_, err := c.Runner.Run(ctx, "funnel", "reset")
	if err != nil {
		return fmt.Errorf("stopping tailscale funnel: %w", err)
	}
	return nil
}

type FunnelStatus map[string]any

func (c Client) FunnelURL(ctx context.Context, fallbackDNS string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := c.Runner.Run(ctx, "funnel", "status", "--json")
	if err == nil {
		if url := findURL(out); url != "" {
			return url, nil
		}
	}
	host := strings.TrimSuffix(fallbackDNS, ".")
	if host == "" {
		return "", errors.New("could not determine Funnel URL")
	}
	return "https://" + host, nil
}

func findURL(data []byte) string {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return ""
	}
	return walkURL(v)
}

func walkURL(v any) string {
	switch x := v.(type) {
	case string:
		if strings.HasPrefix(x, "https://") {
			return x
		}
	case []any:
		for _, item := range x {
			if url := walkURL(item); url != "" {
				return url
			}
		}
	case map[string]any:
		for key, item := range x {
			if strings.Contains(strings.ToLower(key), "url") {
				if url := walkURL(item); url != "" {
					return url
				}
			}
		}
		for _, item := range x {
			if url := walkURL(item); url != "" {
				return url
			}
		}
	}
	return ""
}

func versionAtLeast(version string, major, minor int) bool {
	fields := strings.Fields(version)
	if len(fields) > 0 {
		version = fields[0]
	}
	version = strings.TrimPrefix(version, "v")
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}
	gotMajor, err := strconv.Atoi(numbersOnly(parts[0]))
	if err != nil {
		return false
	}
	gotMinor, err := strconv.Atoi(numbersOnly(parts[1]))
	if err != nil {
		return false
	}
	if gotMajor != major {
		return gotMajor > major
	}
	return gotMinor >= minor
}

func numbersOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

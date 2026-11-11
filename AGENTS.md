# funnelr

A Go CLI and TUI that exposes a local port to the internet with Tailscale Funnel and logs request metadata. It ships through `aayush9029/tap`. Status: shipped.

## Commands

| Task | Command |
|------|---------|
| Build | `go build -o funnelr .` |
| Test | `go test ./...` |

## Architecture

- `internal/ports` scans common local ports for the picker.
- `internal/funnel` starts a background logging proxy (`internal/proxylog`) on a free port. Then it runs `tailscale funnel --bg --yes localhost:<proxy-port>`, so Funnel sends traffic through the proxy.
- `internal/tailscale` wraps the `tailscale` CLI and checks that the machine has the Funnel capability.
- `internal/state` records the active tunnel for `status`, `logs`, and `stop`. Request logs go to `/tmp/funnelr/<port>.log`.
- `internal/clipboard` copies the URL with `pbcopy`, or with OSC 52 inside tmux and other terminals.

## Rules

- The tool needs the Tailscale CLI, an active connection, HTTPS certificates, and Funnel turned on for the machine. Give a clear error when one is missing.
- Log request metadata only. Do not log request or response bodies.
- `funnelr stop` must stop the Funnel and the proxy daemon.

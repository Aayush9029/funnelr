<p align="center">
  <img src="assets/icon.png" width="128" alt="funnelr">
  <h1 align="center">funnelr</h1>
  <p align="center">Expose local web servers with Tailscale Funnel</p>
</p>

<p align="center">
  <a href="https://github.com/Aayush9029/funnelr/releases/latest"><img src="https://img.shields.io/github/v/release/Aayush9029/funnelr" alt="Release"></a>
  <a href="https://github.com/Aayush9029/funnelr/blob/main/LICENSE"><img src="https://img.shields.io/github/license/Aayush9029/funnelr" alt="License"></a>
</p>

## Install

```bash
brew install aayush9029/tap/funnelr
```

Or tap first:

```bash
brew tap aayush9029/tap
brew install funnelr
```

## Usage

```bash
funnelr             # scan common ports and pick one
funnelr 3000        # expose localhost:3000
funnelr status      # show active Funnel URL
funnelr logs        # tail request metadata logs
funnelr stop        # stop the active tunnel
```

`funnelr` requires the Tailscale CLI, an active Tailscale connection, HTTPS certificates, and Funnel enabled for the machine. Request metadata logs are written to `/tmp/funnelr/<port>.log`.

## License

MIT

# sshdeck

Desktop app (Wails v2: Go + React) for SSH sessions, VPN control and read-only server analysis.

## Develop

Needs Go, Node, the `wails` CLI and, on Linux, `pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev`.

```sh
wails dev -tags webkit2_41      # live reload
wails build -tags webkit2_41    # binary in build/bin
go test ./...                   # Go tests
cd frontend && npm test         # UI logic tests
```

Server and VPN metadata live in `~/.config/sshdeck/data.json` (mode 0600). Passwords live only in the OS keyring.

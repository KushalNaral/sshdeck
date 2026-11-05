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

How it fits together: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). Optional organisation mode (not built): [docs/ORG-MODE.md](docs/ORG-MODE.md).

Server and VPN metadata live in `~/.config/sshdeck/data.json` (mode 0600). Passwords live only in the OS keyring (or in the encrypted vault when the passphrase lock is on).

## Bringing your servers in

Admin has an **Import** tab. Save a spreadsheet as CSV (it can hold VPNs, servers and other logins; "Save a blank
template" shows the columns), choose it, review each row and import. A row with the name of something you already
have updates it, and a blank password keeps the stored one. Passwords are read in Go and never shown. "Export all"
writes the same format without passwords.

Each server can also track software versions (PHP, Node, Redis and so on), hold other logins (database, cPanel, RDP;
copy-only, the clipboard clears after 30 seconds), reconnect by itself when the link drops, and connect when
sshdeck starts.

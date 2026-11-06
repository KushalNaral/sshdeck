# sshdeck

A Linux desktop app for keeping SSH logins and VPN profiles in one place. Pick a server and it brings up the VPN if
one is needed, logs in over SSH and opens a terminal. It also reads each server and ranks them by what needs doing.

![sshdeck connected to a server through a VPN](docs/screenshots/overview.jpg)

## Features

- SSH logins by password, key file or ssh-agent; shells open in tabs over one shared connection
- VPN profiles for FortiClient (`openfortivpn`) and GlobalProtect (`gpclient` or `openconnect`). Hillstone has no
  Linux client, so you connect it yourself and confirm in the app.
- `docker exec` shells inside containers
- Files tab: browse a server over SFTP, upload and download with drag and drop, and edit files in an editor tab
- Files and the editor work inside running Docker containers too
- File history: every save keeps the old text, with who changed it and when, so you can compare and restore
- Panels you can resize, hide and dock
- Read-only scan of each server: OS, memory, disks, failed services, containers and software versions
- Servers ranked 1 to 5 by failed services, full disks, outdated software and overdue tasks, with the reasons shown
- Tasks tied to servers
- Reconnects when an SSH link or VPN tunnel drops
- Import and export as CSV
- Passwords in the OS keyring, or in an encrypted vault behind a passphrase; audit log of what was done

![Tasks](docs/screenshots/tasks.jpg)

## Install

Needs Linux with GTK 3 and WebKitGTK 4.1 (`libgtk-3-0` and `libwebkit2gtk-4.1-0` on Debian and Ubuntu).

Download the tarball from the [latest release](../../releases/latest), then:

```sh
tar xzf sshdeck-v*-linux-amd64.tar.gz
cd sshdeck-v*-linux-amd64
./install.sh
```

This checks the files against `SHA256SUMS` and installs into `~/.local/bin` for the current user. Admin, System lists
the VPN clients that are missing and installs them. Starting a VPN asks for your password through `pkexec`.

On Windows, unzip `sshdeck-v*-windows-amd64.zip` and run `sshdeck.exe` (it needs the WebView2 runtime, which Windows 10
and 11 already have). For VPNs, Admin, System has an Install button for OpenConnect, which handles both GlobalProtect
and FortiClient. The first VPN you start in a session asks for administrator approval (UAC); after that sshdeck
reconnects without asking again. Hillstone is still connected by hand.

sshdeck checks GitHub for a newer release when it starts and shows an Update button; Admin, System also has a
"Check for updates" button and a switch for the startup check. Updates are signed and verified before they are installed.

## Usage

1. Admin, VPN profiles: add a profile if some servers are behind a VPN.
2. Admin, Servers: add a server with its host, user and password or key, and pick its VPN.
3. Firefight: select the server and press Connect.

Ctrl+K finds a server, Ctrl+T opens a shell tab, Ctrl+1/2/3 switch between Firefight, Tasks and Admin, and
Ctrl+Shift+F searches the terminal.

To bring in many servers at once, use Admin, Import. "Save a blank template" gives you the CSV columns; each row is
shown as new, update or error before anything is saved, and a blank password keeps the stored one.

The Files tab next to Analysis lists the server's folders. Upload with the button or by dragging files in, and
download the selection. Double-click a file to edit it in a tab (Ctrl+S saves). If the file changed on the server
since you opened it, sshdeck asks before overwriting. Files over 2 MB or that are not text are download-only.

Every save keeps the text it replaced. The History button in the editor lists the versions of a file; click one to
see what changed, and Restore puts it back (as a new version). The History tab next to Files shows who edited what on
the server. By default a copy of the history is also written to `~/.sshdeck/history` on the server so teammates see
each other's edits; production servers need it turned on first. Old versions can hold passwords, so Admin, History
has the settings and a delete button.

Drag the borders between panels to resize them, Ctrl+B hides the server list, Ctrl+. hides the side panel, and
"Dock" moves the side panel next to the server list. "Reset layout" puts everything back.

When the server runs containers, the Files tab gets a picker at the top: choose a container and the same browser,
uploads and editor work on its files. As root it goes through the host's `/proc`; otherwise it uses `docker exec`.
Saved files keep their owner and permissions.

The passphrase lock is in Admin, Security. There is no recovery if the passphrase is lost.

Data lives in `~/.config/sshdeck/`.

## Build from source

Needs Go, Node, the [`wails`](https://wails.io) CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`) and,
on Debian or Ubuntu, `pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev`.

```sh
git clone https://github.com/KushalNaral/sshdeck.git
cd sshdeck
wails build -tags webkit2_41     # binary in build/bin/sshdeck
```

For development use `wails dev -tags webkit2_41`, and run the tests with `go test ./... && (cd frontend && npm test)`.
`./packaging/package.sh v0.3.0` builds the same tarball that the releases carry.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for how it works and [DESIGN.md](DESIGN.md) for the look.

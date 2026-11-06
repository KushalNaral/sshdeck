# sshdeck

A desktop app for Linux and Windows that keeps SSH logins and VPN profiles in one place. Pick a server and it brings
up the VPN if one is needed, logs in over SSH and opens a terminal. It also reads each server and ranks them by what
needs doing.

![sshdeck connected to a server through a VPN](docs/screenshots/overview.jpg)

## Features

- SSH logins by password, key file or ssh-agent; shells open in tabs over one shared connection
- VPN profiles for FortiClient (`openfortivpn`) and GlobalProtect (`gpclient` or `openconnect`). Hillstone has no
  Linux client, so you connect it yourself and confirm in the app. It asks before a second VPN goes up.
- Files: browse over SFTP, drag and drop transfers, an editor, search, and a history of every save you can restore
- The same for running Docker containers, plus `docker exec` shells
- Tunnels (port forwards), jump hosts, and import from CSV or `~/.ssh/config`
- Run one command on several servers, saved snippets, and followed logs
- A read-only scan of each server, with servers ranked 1 to 5 by what needs doing, and tasks tied to servers
- Reconnects when an SSH link or VPN tunnel drops
- Passwords in the OS keyring, or in an encrypted vault behind a passphrase; an audit log of what was done

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

Ctrl+K finds a server, Ctrl+T opens a shell tab, Ctrl+1/2/3/4 switch between Firefight, Tasks, Admin and Run, and
Ctrl+Shift+F searches the terminal.

**Importing.** Admin, Import reads a CSV ("Save a blank template" shows the columns) or the hosts in `~/.ssh/config`,
including `ProxyJump`. Each row shows as new, update or error before anything is saved, and a blank password keeps the
stored one. Hosts from an ssh config log in with the key or the ssh-agent.

**Files.** The Files tab lists the server's folders. Upload with the button or by dragging files in, and double-click a
file to edit it (Ctrl+S saves). If the file changed on the server since you opened it, sshdeck asks before
overwriting. Files over 2 MB or that are not text are download-only. The magnifier searches under the current folder
by name or text, and the terminal button opens a shell there. When the server runs containers, a picker at the top
switches the same tab to a container's files: through the host's `/proc` as root, otherwise through `docker exec`.

**History.** Every save keeps the text it replaced. The History button in the editor lists the versions of a file;
click one to see what changed, and Restore puts it back as a new version. The History tab next to Files shows who
edited what on the server. A copy is also written to `~/.sshdeck/history` on the server so teammates see each other's
edits (production servers need it turned on first). Old versions can hold passwords, so Admin, History has the
settings and a delete button.

**Tunnels and jump hosts.** Add tunnels to a server in Admin, Servers; once connected, the Analysis tab has a
Start/Stop button for each. A local tunnel listens on this machine, so `127.0.0.1:5433` can reach a database that only
the server sees. "Jump through" logs in by way of another server. If you connect a server on a different VPN while
one is up, sshdeck asks whether to switch, because two VPNs at once often break each other's routes.

**Run and logs.** The Run view (Ctrl+4) runs one command on the servers you pick, only the connected ones, and shows
each answer in its own card. Save commands as snippets in Admin, Snippets; each terminal has a Snippets menu that types
one without running it. The scroll button next to the file list follows the selected file like `tail -f`, the one in
the terminal bar follows the system journal, and Analysis has a Log button for each failed service and container.

**Layout.** Drag the borders between panels to resize them. Ctrl+B hides the server list, Ctrl+. hides the side panel,
and "Dock" moves the side panel next to the server list. "Reset layout" puts everything back.

The passphrase lock is in Admin, Security. There is no recovery if the passphrase is lost.

Data lives in `~/.config/sshdeck/` (on Windows, under `%AppData%\sshdeck`).

## Build from source

Needs Go, Node, the [`wails`](https://wails.io) CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`) and,
on Debian or Ubuntu, `pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev`.

```sh
git clone https://github.com/KushalNaral/sshdeck.git
cd sshdeck
wails build -tags webkit2_41     # binary in build/bin/sshdeck
```

For development use `wails dev -tags webkit2_41`, and run the tests with `go test ./... && (cd frontend && npm test)`.
`./packaging/package.sh v0.9.0` builds the same tarball that the releases carry.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for how it works and [DESIGN.md](DESIGN.md) for the look.

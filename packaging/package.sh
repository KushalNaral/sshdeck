#!/bin/sh
# Builds a release and wraps it as dist/sshdeck-<version>-linux-amd64.tar.gz with install.sh inside.
# Needs libwebkit2gtk-4.1 and libgtk-3 on the target machine.
set -e
cd "$(dirname "$0")/.."
v=${1:-$(git describe --tags --always --dirty)}
${WAILS:-wails} build -tags webkit2_41 -clean
git checkout frontend/dist/.gitkeep 2>/dev/null || true
d=dist/sshdeck-$v-linux-amd64
rm -rf "$d" && mkdir -p "$d"
cp build/bin/sshdeck packaging/sshdeck.desktop packaging/sshdeck.svg "$d"
cat > "$d/install.sh" <<'INS'
#!/bin/sh
# Installs for the current user: no root needed. Make sure ~/.local/bin is on your PATH.
set -e
cd "$(dirname "$0")"
install -Dm755 sshdeck ~/.local/bin/sshdeck
install -Dm644 sshdeck.svg ~/.local/share/icons/hicolor/scalable/apps/sshdeck.svg
install -Dm644 sshdeck.desktop ~/.local/share/applications/sshdeck.desktop
echo "Installed. Start sshdeck from your app menu."
INS
chmod +x "$d/install.sh"
tar -C dist -czf "$d.tar.gz" "$(basename "$d")"
echo "$d.tar.gz"

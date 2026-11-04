#!/bin/sh
# Builds a release and wraps it as dist/sshdeck-<version>-linux-amd64.tar.gz.
#   WAILS=~/go/bin/wails   path to the wails CLI
#   SIGN_KEY=<gpg key id>  also writes dist/SHA256SUMS.asc, a detached signature over the checksums
#   SKIP_AUDIT=1           skip the dependency vulnerability check (not recommended)
# The build never passes -devtools, so the shipped app has no inspector. Needs libwebkit2gtk-4.1 and libgtk-3 to run.
set -e
cd "$(dirname "$0")/.."
[ -n "$SKIP_AUDIT" ] || ./packaging/audit.sh
v=${1:-$(git describe --tags --always --dirty)}
${WAILS:-wails} build -tags webkit2_41 -clean
d=dist/sshdeck-$v-linux-amd64
rm -rf "$d" && mkdir -p "$d"
cp build/bin/sshdeck packaging/sshdeck.desktop packaging/sshdeck.svg "$d"
(cd "$d" && sha256sum sshdeck sshdeck.desktop sshdeck.svg > SHA256SUMS)
cat > "$d/install.sh" <<'INS'
#!/bin/sh
# Installs for the current user: no root needed. Make sure ~/.local/bin is on your PATH.
set -e
cd "$(dirname "$0")"
sha256sum -c --quiet SHA256SUMS || { echo "The files do not match SHA256SUMS: not installing." >&2; exit 1; }
install -Dm755 sshdeck ~/.local/bin/sshdeck
install -Dm644 sshdeck.svg ~/.local/share/icons/hicolor/scalable/apps/sshdeck.svg
install -Dm644 sshdeck.desktop ~/.local/share/applications/sshdeck.desktop
echo "Installed. Start sshdeck from your app menu."
INS
chmod +x "$d/install.sh"
tar -C dist -czf "$d.tar.gz" "$(basename "$d")"
(cd dist && sha256sum "$(basename "$d").tar.gz" > SHA256SUMS)
[ -z "$SIGN_KEY" ] || gpg --local-user "$SIGN_KEY" --armor --detach-sign --yes -o dist/SHA256SUMS.asc dist/SHA256SUMS
echo "$d.tar.gz   (checksum: dist/SHA256SUMS${SIGN_KEY:+, signature: dist/SHA256SUMS.asc})"

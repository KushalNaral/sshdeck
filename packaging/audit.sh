#!/bin/sh
# Checks the Go and npm dependencies for known vulnerabilities. Exits non-zero when it finds any,
# or when govulncheck is missing (install: go install golang.org/x/vuln/cmd/govulncheck@latest).
cd "$(dirname "$0")/.."
status=0
gvc=$(command -v govulncheck || echo "$HOME/go/bin/govulncheck")
if [ -x "$gvc" ]; then "$gvc" ./... || status=1; else echo "govulncheck is not installed" >&2; status=1; fi
(cd frontend && npm audit --omit=dev --audit-level=high) || status=1
exit $status

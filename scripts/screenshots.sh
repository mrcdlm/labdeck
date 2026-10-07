#!/bin/sh
# Startet Mock und labdeck mit Beispieldaten und erstellt Screenshots
# (docs/screenshots). Braucht Go, Node und die Playwright-Installation aus e2e/.
#
#	sh scripts/screenshots.sh [Zielverzeichnis]
set -eu
cd "$(dirname "$0")/.."
OUT="${1:-docs/screenshots}"
mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"
TMP="$(mktemp -d)"
trap 'kill $(cat "$TMP"/*.pid 2>/dev/null) 2>/dev/null || true; rm -rf "$TMP"' EXIT

npm --prefix web run build >/dev/null
go build -o "$TMP/server" ./cmd/server
go build -o "$TMP/mock" ./cmd/mock
go build -o "$TMP/devseed" ./cmd/devseed

"$TMP/mock" -addr :8791 >"$TMP/mock.log" 2>&1 & echo $! >"$TMP/mock.pid"
DATA_DIR="$TMP/data" APP_PASSWORD=demo PORT=18081 LOG_LEVEL=warn \
	LABDECK_SECRET_KEY=AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE= \
	"$TMP/server" >"$TMP/server.log" 2>&1 & echo $! >"$TMP/server.pid"
for _ in $(seq 1 50); do
	curl -sf localhost:18081/healthz >/dev/null && curl -sf localhost:8791/_health >/dev/null && break
	sleep 0.2
done

# Integrationen anlegen (über die API), abfragen, dann Verlauf erfinden
node e2e/screenshots.mjs setup http://localhost:18081 http://localhost:8791
"$TMP/devseed" -db "$TMP/data/app.db" >/dev/null 2>&1
node e2e/screenshots.mjs shoot http://localhost:18081 http://localhost:8791 "$OUT"
echo "Screenshots in $OUT"

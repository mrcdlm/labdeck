#!/bin/sh
# Baut Frontend und Server und startet labdeck für die Browser-Tests
# (frische Datenbank in einem temporären Verzeichnis).
set -eu
cd "$(dirname "$0")"

[ -d ../web/node_modules ] || npm --prefix ../web ci
npm --prefix ../web run build >/dev/null
mkdir -p .tmp
go build -o .tmp/server ../cmd/server

DATA_DIR="$(mktemp -d)"
export DATA_DIR
export APP_PASSWORD=test PORT=18080 LOG_LEVEL=info
# Testschlüssel (32 Byte 0x01) – kein echtes Geheimnis
export LABDECK_SECRET_KEY=AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=
exec .tmp/server

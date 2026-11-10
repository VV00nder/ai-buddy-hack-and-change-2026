#!/usr/bin/env bash
# Запуск всех тестов и формирование отчётов о покрытии в ./coverage
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$ROOT/coverage"

echo "== Backend (Go) =="
cd "$ROOT/backend"
go vet ./...
go test ./... -count=1 -coverpkg=./internal/... -coverprofile="$ROOT/coverage/backend.out"
go tool cover -func="$ROOT/coverage/backend.out" | tee "$ROOT/coverage/backend.txt" | tail -1
go tool cover -html="$ROOT/coverage/backend.out" -o "$ROOT/coverage/backend.html"

echo "== Frontend (React/TS) =="
cd "$ROOT/web"
npm run --silent typecheck
npm run --silent coverage | tee "$ROOT/coverage/frontend.txt" | grep -E "all files|# (pass|fail)"
cp coverage/lcov.info "$ROOT/coverage/frontend.lcov.info"

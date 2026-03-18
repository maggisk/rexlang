#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

failed=0

echo "=== Examples ==="
if ! ./rex --test examples/*.rex; then
    failed=1
fi

echo ""
echo "=== Stdlib ==="
# Exclude .browser.rex files — they contain browser-only stubs that can't run natively
stdlib_files=()
for f in internal/stdlib/rexfiles/*.rex internal/stdlib/rexfiles/**/*.rex; do
    case "$f" in *.browser.rex) continue ;; esac
    stdlib_files+=("$f")
done
if ! ./rex --test "${stdlib_files[@]}"; then
    failed=1
fi

echo ""
echo "=== Go tests ==="
if ! go test $(go list ./... | grep -v internal/codegen) ./internal/codegen/ -run 'TestGo'; then
    failed=1
fi

echo ""
if [ "$failed" -eq 0 ]; then
    echo "All tests passed."
else
    echo "Some tests FAILED."
    exit 1
fi

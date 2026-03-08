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
if ! ./rex --test internal/stdlib/rexfiles/*.rex internal/stdlib/rexfiles/**/*.rex; then
    failed=1
fi

echo ""
echo "=== Go tests ==="
if ! go test ./...; then
    failed=1
fi

echo ""
if [ "$failed" -eq 0 ]; then
    echo "All tests passed."
else
    echo "Some tests FAILED."
    exit 1
fi

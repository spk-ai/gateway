#!/usr/bin/env bash
set -eu

echo "=== DevSpace startup ==="

echo "Generating protobuf types..."
buf generate --include-imports

echo "Downloading Go modules..."
go mod download

echo "Starting dev server (air)..."
exec air

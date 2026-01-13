#!/bin/bash
# Build libfray.dylib for macOS
# Called by Xcode Run Script build phase

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
BUILD_DIR="$PROJECT_ROOT/build"

echo "Building libfray.dylib..."

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "error: Go is not installed. Please install Go to build libfray."
    exit 1
fi

# Build the dynamic library
cd "$PROJECT_ROOT"
CGO_ENABLED=1 go build -buildmode=c-shared -o "$BUILD_DIR/libfray.dylib" ./cmd/libfray

echo "Built: $BUILD_DIR/libfray.dylib"

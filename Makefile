VERSION ?= dev
LD_FLAGS := -X github.com/adamavenir/fray/internal/command.Version=$(VERSION)
LD_FLAGS_MCP := -X github.com/adamavenir/fray/cmd/fray-mcp.Version=$(VERSION)

.PHONY: build install test clean dylib

build:
	mkdir -p bin
	go build -ldflags "$(LD_FLAGS)" -o bin/fray ./cmd/fray
	go build -ldflags "$(LD_FLAGS_MCP)" -o bin/fray-mcp ./cmd/fray-mcp

install:
	go install -ldflags "$(LD_FLAGS)" ./cmd/fray
	go install -ldflags "$(LD_FLAGS_MCP)" ./cmd/fray-mcp

test:
	go test ./...

clean:
	rm -rf bin build

dylib:
	mkdir -p build
	CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET=14.0 go build -buildmode=c-shared -o build/libfray.dylib ./cmd/libfray

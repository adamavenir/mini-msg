VERSION ?= dev
LD_FLAGS := -X github.com/adamavenir/mini-msg/internal/command.Version=$(VERSION)
LD_FLAGS_MCP := -X github.com/adamavenir/mini-msg/cmd/mm-mcp.Version=$(VERSION)

.PHONY: build install test clean

build:
	mkdir -p bin
	go build -ldflags "$(LD_FLAGS)" -o bin/mm ./cmd/mm
	go build -ldflags "$(LD_FLAGS_MCP)" -o bin/mm-mcp ./cmd/mm-mcp

install:
	go install -ldflags "$(LD_FLAGS)" ./cmd/mm
	go install -ldflags "$(LD_FLAGS_MCP)" ./cmd/mm-mcp

test:
	go test ./...

clean:
	rm -rf bin

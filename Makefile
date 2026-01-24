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
	@# Check if fray is now accessible in PATH
	@if ! command -v fray >/dev/null 2>&1; then \
		echo ""; \
		echo "⚠️  Installation complete, but 'fray' is not in your PATH."; \
		echo ""; \
		GOBIN=$$(go env GOBIN); \
		if [ -z "$$GOBIN" ]; then \
			GOBIN=$$(go env GOPATH)/bin; \
		fi; \
		echo "Add the Go bin directory to your PATH:"; \
		echo ""; \
		echo "  export PATH=\"$$GOBIN:\$$PATH\""; \
		echo ""; \
		echo "To make this permanent, add that line to your shell config:"; \
		echo "  - ~/.zshrc (zsh)"; \
		echo "  - ~/.bashrc (bash)"; \
		echo ""; \
	fi

test:
	go test ./...

clean:
	rm -rf bin build

dylib:
	mkdir -p build
	CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET=14.0 go build -buildmode=c-shared -o build/libfray.dylib ./cmd/libfray

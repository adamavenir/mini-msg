VERSION ?= dev
LD_FLAGS := -X github.com/adamavenir/mini-msg/internal/command.Version=$(VERSION)

.PHONY: build install test clean

build:
	mkdir -p bin
	go build -ldflags "$(LD_FLAGS)" -o bin/mm ./cmd/mm

install:
	go install -ldflags "$(LD_FLAGS)" ./cmd/mm

test:
	go test ./...

clean:
	rm -rf bin

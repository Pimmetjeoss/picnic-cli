.PHONY: build test lint install clean

build:
	go build -o bin/picnic-pp-cli ./cmd/picnic-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/picnic-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/picnic-pp-mcp ./cmd/picnic-pp-mcp

install-mcp:
	go install ./cmd/picnic-pp-mcp

build-all: build build-mcp

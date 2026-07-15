BINARY_DAEMON=flow
BINARY_CLI=flc
BUILD_DIR=bin

.PHONY: all build build-daemon build-cli clean run-daemon run-cli deps fmt vet test install

all: build

deps:
	go mod tidy
	go mod download

build: build-daemon build-cli

build-daemon:
	go build -o $(BUILD_DIR)/$(BINARY_DAEMON) ./cmd/flow

build-cli:
	go build -o $(BUILD_DIR)/$(BINARY_CLI) ./cmd/flc

run-daemon: build-daemon
	./$(BUILD_DIR)/$(BINARY_DAEMON)

run-cli: build-cli
	./$(BUILD_DIR)/$(BINARY_CLI)

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./... -v

clean:
	rm -rf $(BUILD_DIR)

install: build
	cp $(BUILD_DIR)/$(BINARY_DAEMON) /usr/local/bin/$(BINARY_DAEMON)
	cp $(BUILD_DIR)/$(BINARY_CLI) /usr/local/bin/$(BINARY_CLI)

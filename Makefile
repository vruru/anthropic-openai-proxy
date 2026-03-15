.PHONY: build run clean linux

# Build the proxy binary
build:
	go build -o anthropic-proxy .

# Run the proxy
run:
	go run .

# Clean build artifacts
clean:
	rm -f anthropic-proxy

# Cross-compile for Linux amd64
linux:
	GOOS=linux GOARCH=amd64 go build -o anthropic-proxy-linux-amd64 .

# Build for multiple platforms
cross-compile: linux
	@echo "Cross-compilation complete"

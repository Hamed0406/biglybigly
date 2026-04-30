.PHONY: build build-all clean test lint run dev

# Build for current platform
build:
	go build -o dist/biglybigly ./cmd/biglybigly

# Build for all platforms
build-all: clean
	mkdir -p dist
	# macOS
	GOOS=darwin GOARCH=arm64 go build -o dist/biglybigly-macos-arm64 ./cmd/biglybigly
	GOOS=darwin GOARCH=amd64 go build -o dist/biglybigly-macos-amd64 ./cmd/biglybigly
	# Windows
	GOOS=windows GOARCH=amd64 go build -o dist/biglybigly-windows-amd64.exe ./cmd/biglybigly
	GOOS=windows GOARCH=arm64 go build -o dist/biglybigly-windows-arm64.exe ./cmd/biglybigly
	# Linux
	GOOS=linux GOARCH=amd64 go build -o dist/biglybigly-linux-amd64 ./cmd/biglybigly
	GOOS=linux GOARCH=arm64 go build -o dist/biglybigly-linux-arm64 ./cmd/biglybigly
	GOOS=linux GOARCH=arm go build -o dist/biglybigly-linux-armv7 ./cmd/biglybigly
	@echo "✓ All binaries built to dist/"

# Test
test:
	go test ./...

# Lint
lint:
	go vet ./...
	cd ui && npm run lint

# Run local dev server
run:
	go run ./cmd/biglybigly

# Run UI dev server
dev-ui:
	cd ui && npm run dev

# Clean
clean:
	rm -rf dist/

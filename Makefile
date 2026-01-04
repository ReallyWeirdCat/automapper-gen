# Makefile
.PHONY: build install test clean generate example

# Build the generator
build:
	@echo "Building automapper-gen..."
	go build -o bin/automapper-gen ./cmd/automapper-gen

# Install to GOPATH
install:
	@echo "Installing automapper-gen..."
	go install ./cmd/automapper-gen

# Run all tests
test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -f example/models/automappers.go

# Generate example code
generate: build
	@echo "Generating example mappers..."
	./bin/automapper-gen ./example/models

# Run example
example: generate
	@echo "Running example..."
	cd example && go run main.go

# Format code
fmt:
	@echo "Formatting code..."
	gofmt -s -w .

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run ./...

# Run all checks
check: fmt lint test

# Create release
release:
	@echo "Creating release..."
	goreleaser release --snapshot --clean

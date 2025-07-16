.PHONY: all build test lint clean run-examples help

all: build

build:
	go build ./...

test:
	go test -v ./...

lint:
	golangci-lint run --timeout=5m

clean:
	rm -rf *.db
	rm -f examples/cliexample/cliexample
	rm -f examples/httpexample/httpexample

run-examples:
	@echo "Running CLI example..."
	go run ./examples/cliexample/main.go || true
	@echo "Running HTTP example..."
	go run ./examples/httpexample/main.go || true

help:
	@echo "Available targets:"
	@echo "  build         Build all packages and examples"
	@echo "  test          Run all tests"
	@echo "  lint          Run golangci-lint on the codebase"
	@echo "  clean         Remove generated files and databases"
	@echo "  run-examples  Run CLI and HTTP examples"
	@echo "  help          Show this help message"

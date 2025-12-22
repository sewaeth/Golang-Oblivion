.PHONY: build test coverage lint clean run help

## help: Display this help message
help:
	@echo "Available targets:"
	@echo "  build     - Build the application"
	@echo "  test      - Run all tests with race detection"
	@echo "  coverage  - Generate and view test coverage report"
	@echo "  lint      - Run linters and formatters"
	@echo "  clean     - Remove build artifacts"
	@echo "  run       - Build and run the application"

## build: Build the application binary
build:
	@echo "Building..."
	@go build -o bin/oblivion .

## test: Run tests with race detection
test:
	@echo "Running tests..."
	@go test -v -race -coverprofile=coverage.out ./...

## coverage: Generate coverage report and open in browser
coverage: test
	@echo "Generating coverage report..."
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## lint: Run code formatters and linters
lint:
	@echo "Running formatters and linters..."
	@go fmt ./...
	@go vet ./...
	@echo "Lint complete!"

## clean: Remove build artifacts and coverage files
clean:
	@echo "Cleaning..."
	@rm -f bin/oblivion oblivion coverage.out coverage.html
	@echo "Clean complete!"

## run: Build and run the application
run: build
	@echo "Running application..."
	@./bin/oblivion

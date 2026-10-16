BINARY_NAME = soreaja-printer
MAIN_PACKAGE = .

.PHONY: all build run test clean tidy fmt vet

all: build

build:
	@echo "==> Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) $(MAIN_PACKAGE)

run:
	@echo "==> Running $(BINARY_NAME)..."
	go run $(MAIN_PACKAGE)

test:
	@echo "==> Running tests..."
	go test -v ./...

tidy:
	@echo "==> Tidying dependencies..."
	go mod tidy

fmt:
	@echo "==> Formatting Go files..."
	go fmt ./...

vet:
	@echo "==> Running go vet..."
	go vet ./...

clean:
	@echo "==> Cleaning build artifacts..."
	rm -f $(BINARY_NAME) main *.test *.out

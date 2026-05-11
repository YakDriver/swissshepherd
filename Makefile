.PHONY: build test clean install fmt lint vet tidy deps test-coverage ci help modern modern-check

default: build

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build binary
	@go build -o swissshepherd .

install: ## Install to GOPATH/bin
	@go install .

test: ## Run tests
	@go test ./...

test-coverage: ## Run tests with coverage report
	@go test ./... -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html

vet: ## Run go vet
	@go vet ./...

fmt: ## Format code
	@go fmt ./...

tidy: ## Tidy go.mod
	@go mod tidy

deps: ## Download dependencies
	@go mod download

lint: ## Run golangci-lint
	@golangci-lint run

ci: tidy build test vet ## Run all CI checks locally

modern-check: ## Check for modern Go code
	@echo "make: Checking for modern Go code..."
	@go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest -test ./...

modern: ## Fix modern Go code issues
	@echo "make: Fixing checks for modern Go code..."
	@go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest -fix -test ./...

clean: ## Clean build artifacts
	@rm -f swissshepherd coverage.out coverage.html

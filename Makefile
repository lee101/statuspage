.PHONY: help deps fmt build run test test-go test-e2e clean

PORT ?= 8094
APP_URL ?= http://localhost:$(PORT)
BINARY ?= statuspage

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-14s\033[0m %s\n", $$1, $$2}'

deps: ## Download Go dependencies
	go mod download
	go mod tidy

fmt: ## Format Go code
	gofmt -w *.go

build: fmt ## Build the server binary
	go build -o $(BINARY) .

run: ## Run the development server
	PORT=$(PORT) APP_URL=$(APP_URL) go run .

test-go: ## Run Go tests
	go test ./...

test-e2e: ## Run browser Jasmine e2e tests against ?test=true
	bash scripts/run_e2e.sh

test: build test-go test-e2e ## Run full build and test suite

clean: ## Remove build artifacts and e2e output
	rm -f $(BINARY)
	rm -rf test-results

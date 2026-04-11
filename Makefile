.PHONY: help build run test vet lint tidy clean

GO    ?= go
BIN_DIR := bin
BIN    := $(BIN_DIR)/prism-api

help: ## Show available make targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

build: ## Build the prism-api binary into ./bin
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -o $(BIN) ./cmd/prism-api

run: build ## Build and run the binary locally
	./$(BIN)

test: ## Run unit tests with race detector
	$(GO) test -race -count=1 ./...

vet: ## Run go vet
	$(GO) vet ./...

lint: ## Run golangci-lint (requires golangci-lint in PATH)
	golangci-lint run

tidy: ## Run go mod tidy
	$(GO) mod tidy

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)

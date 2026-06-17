BINARY := sprintmate
PKG := ./cmd/sprintmate

.PHONY: build test cover vet fmt lint run install tidy clean

build: ## Build the binary
	go build -o $(BINARY) $(PKG)

test: ## Run all tests
	go test ./...

cover: ## Run tests with coverage
	go test -coverprofile=coverage.txt ./...
	go tool cover -func=coverage.txt | tail -1

vet: ## Run go vet
	go vet ./...

fmt: ## Format the code
	gofmt -w .

lint: ## Run golangci-lint (must be installed)
	golangci-lint run

run: ## Run from source
	go run $(PKG)

install: ## Install into GOBIN
	go install $(PKG)

tidy: ## Tidy go.mod/go.sum
	go mod tidy

clean: ## Remove build artifacts
	rm -f $(BINARY) $(BINARY).exe coverage.txt
	rm -rf dist

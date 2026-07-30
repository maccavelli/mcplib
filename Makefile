# mcplib — shared library (no binary packaging targets).
# Additive only: does not affect app module Makefiles.
#
# Prefer the user's Go toolchain install (go install ...), then PATH.
GOPATH_BIN    := $(shell go env GOPATH)/bin
GOBIN         := $(shell go env GOBIN)
GOLANGCI_LINT ?= $(GOPATH_BIN)/golangci-lint
GOVULNCHECK   ?= $(or $(wildcard $(GOBIN)/govulncheck),$(GOPATH_BIN)/govulncheck,$(shell command -v govulncheck 2>/dev/null))
GOTESTSUM     ?= $(or $(wildcard $(GOBIN)/gotestsum),$(GOPATH_BIN)/gotestsum,$(shell command -v gotestsum 2>/dev/null))
FLEET_LINT_CFG := .golangci.yml

.PHONY: all help test test-sum fmt vet lint tidy vuln

all: help

test: ## Runs all tests
	go test ./...

test-sum: ## Runs tests via gotestsum (opt-in; requires gotestsum on PATH/GOBIN)
	@if [ -z "$(GOTESTSUM)" ] || [ ! -x "$(GOTESTSUM)" ]; then \
		echo "gotestsum not found. Install: go install gotest.tools/gotestsum@latest"; \
		exit 1; \
	fi
	$(GOTESTSUM) --format testname -- ./...

fmt: ## Formats all Go source files
	go fmt ./...

vet: ## Runs go vet
	go vet ./...

lint: ## Runs golangci-lint with fleet config
	@if [ ! -x "$(GOLANGCI_LINT)" ]; then \
		echo "golangci-lint not found at $(GOLANGCI_LINT)"; \
		echo "Install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"; \
		exit 1; \
	fi
	$(GOLANGCI_LINT) run -c $(FLEET_LINT_CFG) ./...

tidy: ## Runs go mod tidy
	go mod tidy

vuln: ## Runs govulncheck (opt-in; requires govulncheck on PATH/GOBIN)
	@if [ -z "$(GOVULNCHECK)" ] || [ ! -x "$(GOVULNCHECK)" ]; then \
		echo "govulncheck not found. Install: go install golang.org/x/vuln/cmd/govulncheck@latest"; \
		exit 1; \
	fi
	$(GOVULNCHECK) ./...

help: ## Displays this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

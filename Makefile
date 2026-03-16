.SHELL := /bin/bash
.ONESHELL:
MAKEFLAGS += --no-print-directory

BINARY_NAME := raag
TRACKER_NAME := tracker
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GO := go
GO_BUILD_FLAGS := -ldflags="-s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"
GO_RACE_FLAGS := -race
GO_TEST_FLAGS := -v -count=1 -timeout=10m

BIN_DIR := bin
CMD_DIR := cmd
INTERNAL_DIR := internal
TEST_DIR := test

RED := $(shell tput setaf 1 2>/dev/null)
GREEN := $(shell tput setaf 2 2>/dev/null)
YELLOW := $(shell tput setaf 3 2>/dev/null)
BLUE := $(shell tput setaf 4 2>/dev/null)
RESET := $(shell tput sgr0 2>/dev/null)

.PHONY: help \
	all \
	build build-dev build-prod build-race build-tracker \
	clean \
	deps deps-tidy \
	fmt vet lint staticcheck security coverage test test-race test-verbose \
	install run run-tracker docker-build docker-run \
	ci check tidy verify

help:
	@echo "$(BLUE)==============================================$(RESET)"
	@echo "$(BLUE)  Raag P2P Music Streaming$(RESET)"
	@echo "$(BLUE)==============================================$(RESET)"
	@echo ""
	@echo "$(GREEN)Build Targets:$(RESET)"
	@echo "  $(YELLOW)build$(RESET)         - Build production binaries (default)"
	@echo "  $(YELLOW)build-dev$(RESET)     - Build with debug symbols"
	@echo "  $(YELLOW)build-race$(RESET)    - Build with race detector"
	@echo "  $(YELLOW)build-tracker$(RESET)- Build tracker binary"
	@echo ""
	@echo "$(GREEN)Code Quality:$(RESET)"
	@echo "  $(YELLOW)fmt$(RESET)           - Format code (gofumpt)"
	@echo "  $(YELLOW)vet$(RESET)           - Run go vet"
	@echo "  $(YELLOW)lint$(RESET)         - Run all linters (fmt + staticcheck)"
	@echo "  $(YELLOW)staticcheck$(RESET)  - Run staticcheck (strict)"
	@echo "  $(YELLOW)security$(RESET)      - Run security checks (gosec)"
	@echo "  $(YELLOW)coverage$(RESET)     - Run tests with coverage report"
	@echo ""
	@echo "$(GREEN)Testing:$(RESET)"
	@echo "  $(YELLOW)test$(RESET)          - Run all tests"
	@echo "  $(YELLOW)test-race$(RESET)     - Run tests with race detector"
	@echo "  $(YELLOW)test-verbose$(RESET)  - Run tests verbosely"
	@echo ""
	@echo "$(GREEN)Development:$(RESET)"
	@echo "  $(YELLOW)run$(RESET)           - Build and run raag daemon"
	@echo "  $(YELLOW)run-tracker$(RESET)  - Build and run tracker"
	@echo "  $(YELLOW)deps$(RESET)          - Download dependencies"
	@echo "  $(YELLOW)tidy$(RESET)          - Tidy go.mod"
	@echo ""
	@echo "$(GREEN)Docker:$(RESET)"
	@echo "  $(YELLOW)docker-build$(RESET) - Build Docker image"
	@echo "  $(YELLOW)docker-run$(RESET)   - Run Docker container"
	@echo ""
	@echo "$(GREEN)CI/CD:$(RESET)"
	@echo "  $(YELLOW)ci$(RESET)           - Full CI pipeline (deps, fmt, vet, lint, test-race, coverage)"
	@echo "  $(YELLOW)check$(RESET)        - Pre-commit checks"
	@echo ""
	@echo "$(GREEN)Maintenance:$(RESET)"
	@echo "  $(YELLOW)clean$(RESET)         - Remove build artifacts"
	@echo "  $(YELLOW)verify$(RESET)        - Verify dependencies"
	@echo ""

all: deps build build-tracker

build: build-raag build-tracker
	@echo "$(GREEN)Build complete!$(RESET)"

build-raag:
	@echo "$(BLUE)Building $(BINARY_NAME)...$(RESET)"
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GO_BUILD_FLAGS) -o $(BIN_DIR)/$(BINARY_NAME) ./$(CMD_DIR)/raag
	@echo "$(GREEN)✓ Built $(BIN_DIR)/$(BINARY_NAME)$(RESET)"

build-dev: build-raag-dev build-tracker-dev
	@echo "$(GREEN)Dev build complete!$(RESET)"

build-raag-dev:
	@echo "$(BLUE)Building $(BINARY_NAME) (debug)...$(RESET)"
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags="-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)" -o $(BIN_DIR)/$(BINARY_NAME)-dev ./$(CMD_DIR)/raag
	@echo "$(GREEN)✓ Built $(BIN_DIR)/$(BINARY_NAME)-dev$(RESET)"

build-tracker-dev:
	@echo "$(BLUE)Building $(TRACKER_NAME) (debug)...$(RESET)"
	$(GO) build -ldflags="-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)" -o $(BIN_DIR)/$(TRACKER_NAME)-dev ./tracker/cmd/tracker
	@echo "$(GREEN)✓ Built $(BIN_DIR)/$(TRACKER_NAME)-dev$(RESET)"

build-race:
	@echo "$(BLUE)Building with race detector...$(RESET)"
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GO_RACE_FLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-race ./$(CMD_DIR)/raag
	@echo "$(GREEN)✓ Built $(BIN_DIR)/$(BINARY_NAME)-race (with race detector)$(RESET)"

build-tracker:
	@echo "$(BLUE)Building $(TRACKER_NAME)...$(RESET)"
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GO_BUILD_FLAGS) -o $(BIN_DIR)/$(TRACKER_NAME) ./tracker/cmd/tracker
	@echo "$(GREEN)✓ Built $(BIN_DIR)/$(TRACKER_NAME)$(RESET)"

deps: deps-download
	@echo "$(GREEN)Dependencies downloaded!$(RESET)"

deps-download:
	@echo "$(BLUE)Downloading dependencies...$(RESET)"
	$(GO) mod download

deps-tidy: tidy

fmt:
	@echo "$(BLUE)Formatting code with gofumpt...$(RESET)"
	@$(GO) run mvdan.cc/gofumpt@latest -l -w .
	@echo "$(GREEN)✓ Code formatted$(RESET)"

vet:
	@echo "$(BLUE)Running go vet...$(RESET)"
	@$(GO) vet -all ./...
	@echo "$(GREEN)✓ Vet passed$(RESET)"

lint: fmt vet staticcheck
	@echo "$(GREEN)✓ All lint checks passed$(RESET)"

staticcheck:
	@echo "$(BLUE)Running staticcheck...$(RESET)"
	@$(GO) run honnef.co/go/tools/cmd/staticcheck@latest -f stylish ./...
	@echo "$(GREEN)✓ Staticcheck passed$(RESET)"

security:
	@echo "$(BLUE)Running security checks (gosec)...$(RESET)"
	@$(GO) install github.com/securego/gosec/v2/cmd/gosec@latest 2>/dev/null || true
	@gosec -no-fail -quiet ./... || echo "$(YELLOW)⚠ Security warnings found (review above)$(RESET)"
	@echo "$(GREEN)✓ Security scan complete$(RESET)"

test:
	@echo "$(BLUE)Running tests...$(RESET)"
	$(GO) test $(GO_TEST_FLAGS) ./...

test-race:
	@echo "$(BLUE)Running tests with race detector...$(RESET)"
	$(GO) test $(GO_RACE_FLAGS) $(GO_TEST_FLAGS) ./...

test-verbose:
	@echo "$(BLUE)Running verbose tests...$(RESET)"
	$(GO) test -v -v ./...

coverage:
	@echo "$(BLUE)Running tests with coverage...$(RESET)"
	$(GO) test -coverprofile=coverage.out -covermode=atomic ./...
	@echo ""
	@echo "$(BLUE)Coverage Report:$(RESET)"
	$(GO) tool cover -func=coverage.out | tail -n 10
	@echo ""
	@$(GO) tool cover -html=coverage.out -o coverage.html 2>/dev/null || true
	@echo "$(GREEN)✓ Coverage report generated: coverage.out$(RESET)"
	@rm -f coverage.out

run: build-raag
	@echo "$(BLUE)Running $(BINARY_NAME)...$(RESET)"
	./$(BIN_DIR)/$(BINARY_NAME) daemon

run-tracker: build-tracker
	@echo "$(BLUE)Running $(TRACKER_NAME)...$(RESET)"
	./$(BIN_DIR)/$(TRACKER_NAME) --http-port 8080

install:
	@echo "$(BLUE)Installing $(BINARY_NAME)...$(RESET)"
	$(GO) install $(GO_BUILD_FLAGS) ./$(CMD_DIR)/raag
	$(GO) install $(GO_BUILD_FLAGS) ./tracker/cmd/tracker
	@echo "$(GREEN)✓ Installed to \$$GOPATH/bin$(RESET)"

tidy:
	@echo "$(BLUE)Tidying go.mod...$(RESET)"
	$(GO) mod tidy
	@echo "$(GREEN)✓ Dependencies tidied$(RESET)"

docker-build:
	@echo "$(BLUE)Building Docker image...$(RESET)"
	docker build -t raag:$(VERSION) .
	docker build -t raag:latest .
	@echo "$(GREEN)✓ Docker images built$(RESET)"

docker-run:
	@echo "$(BLUE)Running Docker container...$(RESET)"
	docker run -it --rm raag:latest

ci: check test-race coverage
	@echo ""
	@echo "$(GREEN)==============================================$(RESET)"
	@echo "$(GREEN)  CI Pipeline Complete - All Checks Passed!$(RESET)"
	@echo "$(GREEN)==============================================$(RESET)"

check: deps tidy fmt vet lint
	@echo "$(GREEN)✓ Pre-commit checks passed$(RESET)"

verify:
	@echo "$(BLUE)Verifying module...$(RESET)"
	$(GO) mod verify
	@echo "$(GREEN)✓ Module verified$(RESET)"

clean:
	@echo "$(BLUE)Cleaning build artifacts...$(RESET)"
	rm -rf $(BIN_DIR)
	rm -f coverage.out coverage.html
	find . -name "*.test" -delete
	find . -name "*_test.out" -delete
	@echo "$(GREEN)✓ Cleaned!$(RESET)"

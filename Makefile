.PHONY: build run clean test install dev deploy help

# Variables
BINARY_NAME=tglpbot
BUILD_DIR=build
MAIN_FILE=main.go

# Colors for output
GREEN=\033[0;32m
YELLOW=\033[0;33m
RED=\033[0;31m
NC=\033[0m # No Color

help: ## Show this help message
	@echo "$(GREEN)TgLpBot - Telegram Liquidity Pool Bot$(NC)"
	@echo ""
	@echo "$(YELLOW)Available targets:$(NC)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-15s$(NC) %s\n", $$1, $$2}'

install: ## Install dependencies
	@echo "$(GREEN)Installing dependencies...$(NC)"
	go mod download
	go mod tidy
	@echo "$(GREEN)Dependencies installed successfully!$(NC)"

build: ## Build the application
	@echo "$(GREEN)Building $(BINARY_NAME)...$(NC)"
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)
	@echo "$(GREEN)Build complete! Binary: $(BUILD_DIR)/$(BINARY_NAME)$(NC)"

run: ## Run the application
	@echo "$(GREEN)Running $(BINARY_NAME)...$(NC)"
	go run $(MAIN_FILE)

dev: ## Run in development mode with auto-reload (requires air)
	@echo "$(GREEN)Running in development mode...$(NC)"
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "$(RED)Error: 'air' is not installed. Install it with: go install github.com/cosmtrek/air@latest$(NC)"; \
		exit 1; \
	fi

test: ## Run tests
	@echo "$(GREEN)Running tests...$(NC)"
	go test -v ./...

test-coverage: ## Run tests with coverage
	@echo "$(GREEN)Running tests with coverage...$(NC)"
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report generated: coverage.html$(NC)"

clean: ## Clean build artifacts
	@echo "$(YELLOW)Cleaning build artifacts...$(NC)"
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "$(GREEN)Clean complete!$(NC)"

fmt: ## Format code
	@echo "$(GREEN)Formatting code...$(NC)"
	go fmt ./...
	@echo "$(GREEN)Code formatted!$(NC)"

lint: ## Run linter (requires golangci-lint)
	@echo "$(GREEN)Running linter...$(NC)"
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "$(RED)Error: 'golangci-lint' is not installed. Install it from: https://golangci-lint.run/usage/install/$(NC)"; \
		exit 1; \
	fi

vet: ## Run go vet
	@echo "$(GREEN)Running go vet...$(NC)"
	go vet ./...

check: fmt vet lint test ## Run all checks (format, vet, lint, test)
	@echo "$(GREEN)All checks passed!$(NC)"

setup-env: ## Create .env file from example
	@if [ ! -f .env ]; then \
		echo "$(GREEN)Creating .env file from .env.example...$(NC)"; \
		cp .env.example .env; \
		echo "$(YELLOW)Please edit .env file with your configuration!$(NC)"; \
	else \
		echo "$(YELLOW).env file already exists!$(NC)"; \
	fi

setup-db: ## Set up MySQL database
	@echo "$(GREEN)Setting up MySQL database...$(NC)"
	@read -p "Enter MySQL root password: " password; \
	mysql -u root -p$$password -e "CREATE DATABASE IF NOT EXISTS tglpbot CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
	@echo "$(GREEN)Database created successfully!$(NC)"

generate-key: ## Generate encryption key
	@echo "$(GREEN)Generating encryption key...$(NC)"
	@echo "$(YELLOW)Add this to your .env file as ENCRYPTION_KEY:$(NC)"
	@openssl rand -hex 32

docker-build: ## Build Docker image
	@echo "$(GREEN)Building Docker image...$(NC)"
	docker build -t tglpbot:latest .
	@echo "$(GREEN)Docker image built successfully!$(NC)"

docker-run: ## Run Docker container
	@echo "$(GREEN)Running Docker container...$(NC)"
	docker run --env-file .env -d --name tglpbot tglpbot:latest
	@echo "$(GREEN)Container started!$(NC)"

docker-stop: ## Stop Docker container
	@echo "$(YELLOW)Stopping Docker container...$(NC)"
	docker stop tglpbot
	docker rm tglpbot
	@echo "$(GREEN)Container stopped!$(NC)"

deploy: build ## Deploy to production
	@echo "$(GREEN)Deploying to production...$(NC)"
	@echo "$(YELLOW)Make sure to:$(NC)"
	@echo "  1. Update .env with production values"
	@echo "  2. Deploy Zap contract to BSC mainnet"
	@echo "  3. Set up systemd service"
	@echo "  4. Configure firewall rules"
	@echo ""
	@echo "$(GREEN)Binary ready at: $(BUILD_DIR)/$(BINARY_NAME)$(NC)"

.DEFAULT_GOAL := help


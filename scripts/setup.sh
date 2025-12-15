#!/bin/bash

# TgLpBot Setup Script
# This script helps set up the development environment

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}TgLpBot Setup Script${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed!${NC}"
    echo -e "${YELLOW}Please install Go 1.21 or higher from https://golang.org/dl/${NC}"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo -e "${GREEN}Go version: $GO_VERSION${NC}"
echo ""

# Check if MySQL is installed
if ! command -v mysql &> /dev/null; then
    echo -e "${YELLOW}Warning: MySQL client not found!${NC}"
    echo -e "${YELLOW}Please install MySQL 8.0+ manually${NC}"
else
    echo -e "${GREEN}MySQL client found${NC}"
fi

# Check if Redis is installed
if ! command -v redis-cli &> /dev/null; then
    echo -e "${YELLOW}Warning: Redis client not found!${NC}"
    echo -e "${YELLOW}Please install Redis 6.0+ manually${NC}"
else
    echo -e "${GREEN}Redis client found${NC}"
fi

echo ""

# Create .env file if it doesn't exist
if [ ! -f .env ]; then
    echo -e "${GREEN}Creating .env file...${NC}"
    cp .env.example .env
    
    # Generate encryption key
    ENCRYPTION_KEY=$(openssl rand -hex 32)
    
    # Update .env with generated key
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        sed -i '' "s/your_32_byte_hex_encryption_key_here/$ENCRYPTION_KEY/" .env
    else
        # Linux
        sed -i "s/your_32_byte_hex_encryption_key_here/$ENCRYPTION_KEY/" .env
    fi
    
    echo -e "${GREEN}.env file created!${NC}"
    echo -e "${YELLOW}Please edit .env file with your configuration:${NC}"
    echo -e "  - TELEGRAM_BOT_TOKEN"
    echo -e "  - MYSQL_PASSWORD"
    echo -e "  - OKX API credentials (optional)"
    echo ""
else
    echo -e "${YELLOW}.env file already exists, skipping...${NC}"
    echo ""
fi

# Install Go dependencies
echo -e "${GREEN}Installing Go dependencies...${NC}"
go mod download
go mod tidy
echo -e "${GREEN}Dependencies installed!${NC}"
echo ""

# Create database
read -p "Do you want to create the MySQL database? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    source .env
    echo -e "${GREEN}Creating database...${NC}"
    mysql -u root -p -e "CREATE DATABASE IF NOT EXISTS $MYSQL_DATABASE CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
    echo -e "${GREEN}Database created!${NC}"
fi

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Setup complete!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo -e "  1. Edit .env file with your configuration"
echo -e "  2. Deploy ZapV3V4Improved to BSC (see contracts/README.md)"
echo -e "  3. Update ZAP_V3_ADDRESS / ZAP_V4_ADDRESS in .env"
echo -e "  4. Run the bot with: ${GREEN}go run main.go${NC}"
echo ""
echo -e "${YELLOW}Useful commands:${NC}"
echo -e "  Build:            ${GREEN}go build -o tglpbot main.go${NC}"
echo -e "  Run:              ${GREEN}go run main.go${NC}"
echo -e "  Test:             ${GREEN}go test ./...${NC}"
echo -e "  Contracts deploy: ${GREEN}cd contracts && npm run deploy:mainnet${NC}"
echo ""

#!/bin/bash

# TgLpBot Deployment Script
# This script helps deploy the bot to a production server

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}TgLpBot Deployment Script${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Check if .env exists
if [ ! -f .env ]; then
    echo -e "${RED}Error: .env file not found!${NC}"
    echo -e "${YELLOW}Please create .env file from .env.example${NC}"
    exit 1
fi

# Load environment variables
source .env

# Check required variables
if [ -z "$TELEGRAM_BOT_TOKEN" ]; then
    echo -e "${RED}Error: TELEGRAM_BOT_TOKEN not set in .env${NC}"
    exit 1
fi

if [ -z "$ENCRYPTION_KEY" ]; then
    echo -e "${RED}Error: ENCRYPTION_KEY not set in .env${NC}"
    echo -e "${YELLOW}Generate one with: openssl rand -hex 32${NC}"
    exit 1
fi

# Build the application
echo -e "${GREEN}Building application...${NC}"
make build

if [ $? -ne 0 ]; then
    echo -e "${RED}Build failed!${NC}"
    exit 1
fi

echo -e "${GREEN}Build successful!${NC}"
echo ""

# Check if systemd service exists
if [ -f /etc/systemd/system/tglpbot.service ]; then
    echo -e "${YELLOW}Stopping existing service...${NC}"
    sudo systemctl stop tglpbot
fi

# Copy binary to /usr/local/bin
echo -e "${GREEN}Installing binary...${NC}"
sudo cp build/tglpbot /usr/local/bin/tglpbot
sudo chmod +x /usr/local/bin/tglpbot

# Create systemd service
echo -e "${GREEN}Creating systemd service...${NC}"
sudo tee /etc/systemd/system/tglpbot.service > /dev/null <<EOF
[Unit]
Description=Telegram LP Bot
After=network.target mysql.service redis.service

[Service]
Type=simple
User=$USER
WorkingDirectory=$(pwd)
ExecStart=/usr/local/bin/tglpbot
Restart=always
RestartSec=10
StandardOutput=append:/var/log/tglpbot/output.log
StandardError=append:/var/log/tglpbot/error.log

[Install]
WantedBy=multi-user.target
EOF

# Create log directory
sudo mkdir -p /var/log/tglpbot
sudo chown $USER:$USER /var/log/tglpbot

# Reload systemd
echo -e "${GREEN}Reloading systemd...${NC}"
sudo systemctl daemon-reload

# Enable service
echo -e "${GREEN}Enabling service...${NC}"
sudo systemctl enable tglpbot

# Start service
echo -e "${GREEN}Starting service...${NC}"
sudo systemctl start tglpbot

# Check status
sleep 2
if sudo systemctl is-active --quiet tglpbot; then
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}Deployment successful!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo -e "${GREEN}Service status:${NC}"
    sudo systemctl status tglpbot --no-pager
    echo ""
    echo -e "${YELLOW}Useful commands:${NC}"
    echo -e "  View logs:        ${GREEN}sudo journalctl -u tglpbot -f${NC}"
    echo -e "  Stop service:     ${GREEN}sudo systemctl stop tglpbot${NC}"
    echo -e "  Start service:    ${GREEN}sudo systemctl start tglpbot${NC}"
    echo -e "  Restart service:  ${GREEN}sudo systemctl restart tglpbot${NC}"
    echo -e "  Service status:   ${GREEN}sudo systemctl status tglpbot${NC}"
else
    echo -e "${RED}========================================${NC}"
    echo -e "${RED}Deployment failed!${NC}"
    echo -e "${RED}========================================${NC}"
    echo ""
    echo -e "${YELLOW}Check logs with:${NC}"
    echo -e "  ${GREEN}sudo journalctl -u tglpbot -n 50${NC}"
    exit 1
fi


#!/bin/bash
# Production deployment script for Karl Media Server

# Exit on error
set -e

# Default values
CONFIG_DIR="/etc/karl"
RUN_DIR="/var/run/karl"
LOG_DIR="/var/log/karl"
INSTALL_DIR="/opt/karl"
USER="karl"
GROUP="karl"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --config-dir=*)
      CONFIG_DIR="${1#*=}"
      shift
      ;;
    --run-dir=*)
      RUN_DIR="${1#*=}"
      shift
      ;;
    --log-dir=*)
      LOG_DIR="${1#*=}"
      shift
      ;;
    --install-dir=*)
      INSTALL_DIR="${1#*=}"
      shift
      ;;
    --user=*)
      USER="${1#*=}"
      shift
      ;;
    --group=*)
      GROUP="${1#*=}"
      shift
      ;;
    --help)
      echo "Karl Media Server - Deployment Script"
      echo ""
      echo "Usage: $0 [options]"
      echo ""
      echo "Options:"
      echo "  --config-dir=DIR   Configuration directory (default: /etc/karl)"
      echo "  --run-dir=DIR      Runtime directory (default: /var/run/karl)"
      echo "  --log-dir=DIR      Log directory (default: /var/log/karl)"
      echo "  --install-dir=DIR  Installation directory (default: /opt/karl)"
      echo "  --user=USER        User to run Karl as (default: karl)"
      echo "  --group=GROUP      Group to run Karl as (default: karl)"
      echo "  --help             Show this help message"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

# Display deployment information
echo "Karl Media Server - Production Deployment"
echo "----------------------------------------"
echo "Configuration directory: $CONFIG_DIR"
echo "Runtime directory: $RUN_DIR"
echo "Log directory: $LOG_DIR"
echo "Installation directory: $INSTALL_DIR"
echo "User: $USER"
echo "Group: $GROUP"
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
  echo "Please run as root or with sudo"
  exit 1
fi

# Build the application
echo "Building Karl Media Server..."
go build -o karl

# Create user and group if they don't exist
echo "Creating user and group if needed..."
if ! getent group $GROUP >/dev/null; then
  groupadd --system $GROUP
fi

if ! getent passwd $USER >/dev/null; then
  useradd --system --gid $GROUP --no-create-home --shell /usr/sbin/nologin $USER
fi

# Create directories
echo "Creating directories..."
mkdir -p $CONFIG_DIR
mkdir -p $RUN_DIR
mkdir -p $LOG_DIR
mkdir -p $INSTALL_DIR

# Copy configuration
echo "Copying configuration..."
if [ ! -f "$CONFIG_DIR/config.json" ]; then
  cp config/config.json $CONFIG_DIR/config.json
fi

# Copy binary
echo "Installing binary..."
cp karl $INSTALL_DIR/

# Set permissions
echo "Setting permissions..."
chown -R $USER:$GROUP $CONFIG_DIR
chown -R $USER:$GROUP $RUN_DIR
chown -R $USER:$GROUP $LOG_DIR
chown -R $USER:$GROUP $INSTALL_DIR
chmod 755 $INSTALL_DIR/karl

# Create systemd service
echo "Creating systemd service..."
cat > /etc/systemd/system/karl.service << EOL
[Unit]
Description=Karl Media Server
After=network.target
After=mysql.service
After=redis.service

[Service]
Type=simple
User=${USER}
Group=${GROUP}
ExecStart=${INSTALL_DIR}/karl
WorkingDirectory=${INSTALL_DIR}
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=karl
Environment=KARL_CONFIG_PATH=${CONFIG_DIR}/config.json
Environment=KARL_LOG_LEVEL=3
Environment=KARL_RUN_DIR=${RUN_DIR}

[Install]
WantedBy=multi-user.target
EOL

# Reload systemd
echo "Reloading systemd..."
systemctl daemon-reload

# Start service
echo "Starting Karl Media Server..."
systemctl enable karl
systemctl start karl

echo ""
echo "Karl Media Server has been deployed successfully!"
echo ""
echo "To check status: systemctl status karl"
echo "To view logs: journalctl -u karl"
echo ""
echo "For more information, see the documentation:"
echo "- README.md - Overview"
echo "- DOCUMENTATION.md - Detailed configuration"
echo "- PRODUCTION-READY.md - Production best practices"
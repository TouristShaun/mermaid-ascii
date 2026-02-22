#!/usr/bin/env bash
# setup.sh — Bootstrap a fresh DigitalOcean droplet for Forge.
#
# Usage:
#   ssh root@your-droplet "bash -s" < setup.sh
#
# Prerequisites:
#   - Ubuntu 22.04+ droplet
#   - SSH access as root
#   - A domain pointed at the droplet's IP

set -euo pipefail

echo "=== Forge Droplet Setup ==="

# Update system.
apt-get update && apt-get upgrade -y

# Install Docker.
if ! command -v docker &> /dev/null; then
    curl -fsSL https://get.docker.com | sh
    systemctl enable docker
    systemctl start docker
fi

# Install Docker Compose plugin.
if ! docker compose version &> /dev/null; then
    apt-get install -y docker-compose-plugin
fi

# Install git.
apt-get install -y git

# Create forge user.
if ! id forge &> /dev/null; then
    useradd -m -s /bin/bash forge
    usermod -aG docker forge
fi

# Create data directories.
mkdir -p /var/lib/forge/repos
chown -R forge:forge /var/lib/forge

# Clone the repo (or you can scp the deploy files).
DEPLOY_DIR=/home/forge/deploy
mkdir -p "$DEPLOY_DIR"

echo ""
echo "=== Setup complete ==="
echo ""
echo "Next steps:"
echo "  1. Copy your deploy/ directory to $DEPLOY_DIR"
echo "  2. Edit $DEPLOY_DIR/droplet/Caddyfile with your domain"
echo "  3. cd $DEPLOY_DIR/droplet && docker compose up -d"
echo "  4. From your MacBook: hive start -f instructions.yaml -forge https://forge.yourdomain.com"
echo ""

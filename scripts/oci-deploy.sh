#!/bin/bash
# HyperCache OCI Free Tier Deployment Script
# Run this on an Oracle Linux / Ubuntu VM via SSH
#
# Usage:
#   ssh opc@<PUBLIC_IP>
#   curl -sSL https://raw.githubusercontent.com/rishabhverma17/HyperCache/main/scripts/oci-deploy.sh | bash
#
# Or copy this file and run: chmod +x oci-deploy.sh && ./oci-deploy.sh

set -euo pipefail

echo "=== HyperCache OCI Deployment ==="
echo ""

# Detect OS
if [ -f /etc/oracle-release ] || [ -f /etc/redhat-release ]; then
    PKG_MANAGER="dnf"
    echo "Detected: Oracle Linux / RHEL"
elif [ -f /etc/lsb-release ]; then
    PKG_MANAGER="apt"
    echo "Detected: Ubuntu"
else
    echo "Unsupported OS. Use Oracle Linux or Ubuntu."
    exit 1
fi

# Step 1: Install Docker
echo ""
echo "=== Step 1: Installing Docker ==="
if command -v docker &> /dev/null; then
    echo "Docker already installed: $(docker --version)"
else
    if [ "$PKG_MANAGER" = "dnf" ]; then
        sudo dnf install -y dnf-utils
        sudo dnf config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
        sudo dnf install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
    else
        sudo apt-get update
        sudo apt-get install -y ca-certificates curl gnupg
        sudo install -m 0755 -d /etc/apt/keyrings
        curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
        sudo chmod a+r /etc/apt/keyrings/docker.gpg
        echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
        sudo apt-get update
        sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
    fi

    sudo systemctl enable docker
    sudo systemctl start docker
    sudo usermod -aG docker $USER
    echo "Docker installed. You may need to log out and back in for group changes."
fi

# Step 2: Open firewall ports (OCI Linux firewall)
echo ""
echo "=== Step 2: Opening firewall ports ==="
if command -v firewall-cmd &> /dev/null; then
    sudo firewall-cmd --permanent --add-port=9080/tcp  # HTTP API node 1
    sudo firewall-cmd --permanent --add-port=9081/tcp  # HTTP API node 2
    sudo firewall-cmd --permanent --add-port=9082/tcp  # HTTP API node 3
    sudo firewall-cmd --permanent --add-port=8080/tcp  # RESP node 1
    sudo firewall-cmd --permanent --add-port=8081/tcp  # RESP node 2
    sudo firewall-cmd --permanent --add-port=8082/tcp  # RESP node 3
    sudo firewall-cmd --permanent --add-port=3000/tcp  # Grafana
    sudo firewall-cmd --permanent --add-port=9200/tcp  # Elasticsearch
    sudo firewall-cmd --reload
    echo "Firewall ports opened."
elif command -v ufw &> /dev/null; then
    sudo ufw allow 9080/tcp
    sudo ufw allow 9081/tcp
    sudo ufw allow 9082/tcp
    sudo ufw allow 8080/tcp
    sudo ufw allow 3000/tcp
    echo "UFW ports opened."
else
    echo "No firewall manager found. Ports may already be open."
fi

# Step 3: Download docker-compose file
echo ""
echo "=== Step 3: Downloading HyperCache docker-compose ==="
mkdir -p ~/hypercache && cd ~/hypercache

curl -sSL -o docker-compose.cluster.yml \
    https://raw.githubusercontent.com/rishabhverma17/HyperCache/main/docker-compose.cluster.yml

echo "Downloaded docker-compose.cluster.yml"

# Step 4: Start the cluster
echo ""
echo "=== Step 4: Starting HyperCache cluster ==="
# Need to use sudo if user not yet in docker group (fresh install)
if groups | grep -q docker; then
    docker compose -f docker-compose.cluster.yml up -d
else
    sudo docker compose -f docker-compose.cluster.yml up -d
fi

echo ""
echo "=== Step 5: Waiting for services to be healthy ==="
sleep 15

# Step 6: Verify
echo ""
echo "=== Deployment Complete! ==="
echo ""
echo "Public IP: $(curl -s ifconfig.me 2>/dev/null || echo '<check OCI console>')"
echo ""
echo "Endpoints:"
echo "  HTTP API (Node 1): http://$(curl -s ifconfig.me):9080/health"
echo "  HTTP API (Node 2): http://$(curl -s ifconfig.me):9081/health"
echo "  HTTP API (Node 3): http://$(curl -s ifconfig.me):9082/health"
echo "  RESP (Redis CLI):  redis-cli -h $(curl -s ifconfig.me) -p 8080"
echo "  Grafana:           http://$(curl -s ifconfig.me):3000 (admin / admin123)"
echo ""
echo "Quick test:"
echo "  curl http://$(curl -s ifconfig.me):9080/health"
echo ""
echo "  curl -X PUT http://$(curl -s ifconfig.me):9080/api/cache/hello \\"
echo "    -H 'Content-Type: application/json' -d '{\"value\": \"world\"}'"
echo ""
echo "  curl http://$(curl -s ifconfig.me):9082/api/cache/hello  # read from node 3"

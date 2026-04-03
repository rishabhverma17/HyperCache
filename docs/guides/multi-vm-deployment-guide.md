# HyperCache Multi-VM/Docker Deployment Guide

## Overview

This guide explains how to deploy HyperCache across multiple Virtual Machines (VMs) or Docker containers in a distributed environment like Azure, AWS, or any cloud provider.

## Table of Contents
- [Network Architecture](#network-architecture)
- [Configuration Requirements](#configuration-requirements)
- [Azure VM Deployment](#azure-vm-deployment)
- [Docker Deployment](#docker-deployment)
- [Kubernetes Deployment](#kubernetes-deployment)
- [Network Security](#network-security)
- [Troubleshooting](#troubleshooting)

---

## Network Architecture

### Current Cluster Formation Mechanism

HyperCache uses **Hashicorp Serf** for gossip-based membership management:

```go
// internal/cluster/gossip_membership.go
// Serf handles:
// - Node discovery and membership
// - Failure detection
// - Cluster state synchronization
// - Network partition handling
```

### Required Network Configuration

#### Ports Used by HyperCache:
| Service | Port | Protocol | Purpose |
|---------|------|----------|---------|
| RESP API | 8080-8082 | TCP | Redis protocol interface |
| HTTP API | 9080-9082 | TCP | REST API interface |
| Serf Gossip | 7946 | TCP+UDP | Cluster membership |
| Serf RPC | 7373 | TCP | Internal cluster communication |

### Multi-VM Network Requirements

```yaml
# Network connectivity matrix required:
VM1 (10.0.1.4) ←→ VM2 (10.0.1.5) ←→ VM3 (10.0.1.6)
     ↓                 ↓                 ↓
   Ports:           Ports:           Ports:
   - 8080 (RESP)    - 8081 (RESP)    - 8082 (RESP)
   - 9080 (HTTP)    - 9081 (HTTP)    - 9082 (HTTP)
   - 7946 (Gossip)  - 7946 (Gossip)  - 7946 (Gossip)
   - 7373 (RPC)     - 7373 (RPC)     - 7373 (RPC)
```

---

## Configuration Requirements

### 1. Update Configuration for Multi-VM

**Current Issue**: Configuration assumes localhost deployment
**Solution**: Make network addresses configurable

#### Enhanced Configuration Structure:

```yaml
# configs/hypercache-vm.yaml
server:
  node_id: "node-1"
  
network:
  # Bind addresses (what this node listens on)
  resp_bind_addr: "0.0.0.0"      # Listen on all interfaces
  resp_port: 8080
  
  http_bind_addr: "0.0.0.0"      # Listen on all interfaces  
  http_port: 9080
  
  # Advertise addresses (what other nodes use to connect)
  advertise_addr: "10.0.1.4"     # VM's actual IP
  gossip_port: 7946
  
cluster:
  # Seed nodes for initial cluster formation
  seed_nodes:
    - "10.0.1.4:7946"  # VM1
    - "10.0.1.5:7946"  # VM2
    - "10.0.1.6:7946"  # VM3
  
  replication_factor: 3
  
persistence:
  enabled: true
  data_dir: "/opt/hypercache/data"
  
logging:
  level: "INFO"
  format: "json"
```

### 2. Required Code Changes

✅ **Configuration-Driven Networking**: The code now reads network settings from configuration files, not hardcoded values.

#### ✅ Configuration Structure (Already Implemented):
```yaml
# configs/hypercache-vm.yaml
network:
  resp_bind_addr: "0.0.0.0"      # Bind to all interfaces
  resp_port: 8080                # RESP protocol port
  http_bind_addr: "0.0.0.0"      # HTTP API bind address  
  http_port: 9080                # HTTP API port
  advertise_addr: "10.0.1.4"     # VM's actual IP for cluster communication
  gossip_port: 7946              # Serf gossip port

cluster:
  seeds:                         # Seed nodes for cluster formation
    - "10.0.1.4:7946"           # VM1 gossip address
    - "10.0.1.5:7946"           # VM2 gossip address
    - "10.0.1.6:7946"           # VM3 gossip address
```

#### ✅ Automatic Configuration Usage (Already Implemented):
```go
// cmd/hypercache/main.go automatically uses:
// - cfg.Network.RESPBindAddr and cfg.Network.RESPPort for RESP server
// - cfg.Network.HTTPBindAddr and cfg.Network.HTTPPort for HTTP API
// - cfg.Network.AdvertiseAddr for cluster communication
// - cfg.Network.GossipPort for Serf gossip protocol
// - cfg.Cluster.Seeds for joining existing cluster
```

---

## Azure VM Deployment

### 1. VM Setup

#### Create Azure VMs:
```bash
# Azure CLI commands
az group create --name hypercache-rg --location eastus

# Create 3 VMs in the same subnet
for i in {1..3}; do
  az vm create \
    --resource-group hypercache-rg \
    --name hypercache-vm-$i \
    --image Ubuntu2004 \
    --size Standard_D2s_v3 \
    --vnet-name hypercache-vnet \
    --subnet hypercache-subnet \
    --nsg hypercache-nsg \
    --public-ip-sku Standard \
    --admin-username hypercache \
    --generate-ssh-keys
done
```

#### Configure Network Security Group:
```bash
# Allow required ports
az network nsg rule create \
  --resource-group hypercache-rg \
  --nsg-name hypercache-nsg \
  --name AllowHyperCachePorts \
  --protocol Tcp \
  --priority 1000 \
  --source-address-prefixes VirtualNetwork \
  --destination-port-ranges 8080-8082 9080-9082 7946 7373

# Allow gossip UDP
az network nsg rule create \
  --resource-group hypercache-rg \
  --nsg-name hypercache-nsg \
  --name AllowGossipUDP \
  --protocol Udp \
  --priority 1001 \
  --source-address-prefixes VirtualNetwork \
  --destination-port-ranges 7946
```

### 2. Software Installation

#### On Each VM:
```bash
# Install Go
wget https://go.dev/dl/go1.23.2.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.23.2.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Clone and build HyperCache
git clone <your-repo> /opt/hypercache
cd /opt/hypercache
go build -o bin/hypercache cmd/hypercache/main.go

# Create directories
sudo mkdir -p /opt/hypercache/data
sudo mkdir -p /opt/hypercache/logs
sudo chown -R hypercache:hypercache /opt/hypercache
```

### 3. Configuration for Each VM

#### VM1 (10.0.1.4) - configs/vm1-config.yaml:
```yaml
server:
  node_id: "vm1-node"
  
network:
  resp_bind_addr: "0.0.0.0"
  resp_port: 8080
  http_bind_addr: "0.0.0.0"
  http_port: 9080
  advertise_addr: "10.0.1.4"
  gossip_port: 7946
  
cluster:
  seed_nodes:
    - "10.0.1.4:7946"
    - "10.0.1.5:7946"  
    - "10.0.1.6:7946"
```

#### VM2 (10.0.1.5) - configs/vm2-config.yaml:
```yaml
server:
  node_id: "vm2-node"
  
network:
  resp_bind_addr: "0.0.0.0"
  resp_port: 8081
  http_bind_addr: "0.0.0.0"
  http_port: 9081
  advertise_addr: "10.0.1.5"
  gossip_port: 7946
  
cluster:
  seed_nodes:
    - "10.0.1.4:7946"
    - "10.0.1.5:7946"
    - "10.0.1.6:7946"
```

#### VM3 (10.0.1.6) - configs/vm3-config.yaml:
```yaml
server:
  node_id: "vm3-node"
  
network:
  resp_bind_addr: "0.0.0.0"
  resp_port: 8082
  http_bind_addr: "0.0.0.0"
  http_port: 9082
  advertise_addr: "10.0.1.6"
  gossip_port: 7946
  
cluster:
  seed_nodes:
    - "10.0.1.4:7946"
    - "10.0.1.5:7946"
    - "10.0.1.6:7946"
```

### 4. Service Setup

#### Create systemd service on each VM:
```bash
# /etc/systemd/system/hypercache.service
[Unit]
Description=HyperCache Distributed Cache
After=network.target

[Service]
Type=simple
User=hypercache
WorkingDirectory=/opt/hypercache
ExecStart=/opt/hypercache/bin/hypercache --config=/opt/hypercache/configs/vm1-config.yaml
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

#### Start services:
```bash
sudo systemctl daemon-reload
sudo systemctl enable hypercache
sudo systemctl start hypercache

# Check status
sudo systemctl status hypercache
journalctl -u hypercache -f
```

---

## Docker Deployment

### 1. Dockerfile

```dockerfile
# Dockerfile
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY . .
RUN go build -o hypercache cmd/hypercache/main.go

FROM alpine:3.18
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/hypercache .
COPY --from=builder /app/configs ./configs/

# Create data directory
RUN mkdir -p /data

# Expose ports
EXPOSE 8080 9080 7946/tcp 7946/udp 7373

CMD ["./hypercache"]
```

### 2. Docker Compose for Multi-Host

#### docker-compose.yml:
```yaml
version: '3.8'

services:
  hypercache-node1:
    build: .
    hostname: hypercache-node1
    environment:
      - NODE_ID=docker-node1
      - BIND_ADDR=0.0.0.0
      - ADVERTISE_ADDR=hypercache-node1
      - SEED_NODES=hypercache-node1:7946,hypercache-node2:7946,hypercache-node3:7946
    ports:
      - "8080:8080"
      - "9080:9080"
    volumes:
      - node1_data:/data
    networks:
      - hypercache-network

  hypercache-node2:
    build: .
    hostname: hypercache-node2
    environment:
      - NODE_ID=docker-node2
      - BIND_ADDR=0.0.0.0
      - ADVERTISE_ADDR=hypercache-node2
      - SEED_NODES=hypercache-node1:7946,hypercache-node2:7946,hypercache-node3:7946
    ports:
      - "8081:8080"
      - "9081:9080"
    volumes:
      - node2_data:/data
    networks:
      - hypercache-network

  hypercache-node3:
    build: .
    hostname: hypercache-node3
    environment:
      - NODE_ID=docker-node3
      - BIND_ADDR=0.0.0.0
      - ADVERTISE_ADDR=hypercache-node3
      - SEED_NODES=hypercache-node1:7946,hypercache-node2:7946,hypercache-node3:7946
    ports:
      - "8082:8080"
      - "9082:9080"
    volumes:
      - node3_data:/data
    networks:
      - hypercache-network

volumes:
  node1_data:
  node2_data:
  node3_data:

networks:
  hypercache-network:
    driver: bridge
```

### 3. Multi-Host Docker Deployment

#### Using Docker Swarm:
```bash
# Initialize swarm on manager node
docker swarm init --advertise-addr 10.0.1.4

# Join workers
docker swarm join --token <token> 10.0.1.4:2377

# Deploy stack
docker stack deploy -c docker-compose.yml hypercache
```

#### Using Docker Network Overlay:
```bash
# Create overlay network
docker network create --driver overlay hypercache-network

# Run containers on different hosts
# Host 1:
docker run -d --name hypercache-node1 \
  --network hypercache-network \
  -p 8080:8080 -p 9080:9080 \
  -e NODE_ID=docker-node1 \
  -e ADVERTISE_ADDR=10.0.1.4 \
  hypercache:latest

# Host 2:
docker run -d --name hypercache-node2 \
  --network hypercache-network \
  -p 8081:8080 -p 9081:9080 \
  -e NODE_ID=docker-node2 \
  -e ADVERTISE_ADDR=10.0.1.5 \
  hypercache:latest

# Host 3:
docker run -d --name hypercache-node3 \
  --network hypercache-network \
  -p 8082:8080 -p 9082:9080 \
  -e NODE_ID=docker-node3 \
  -e ADVERTISE_ADDR=10.0.1.6 \
  hypercache:latest
```

---

## Kubernetes Deployment

### 1. ConfigMap

```yaml
# k8s/configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: hypercache-config
data:
  config.yaml: |
    cluster:
      replication_factor: 3
    persistence:
      enabled: true
      data_dir: "/data"
    logging:
      level: "INFO"
      format: "json"
```

### 2. StatefulSet

```yaml
# k8s/statefulset.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: hypercache
spec:
  serviceName: hypercache-headless
  replicas: 3
  selector:
    matchLabels:
      app: hypercache
  template:
    metadata:
      labels:
        app: hypercache
    spec:
      containers:
      - name: hypercache
        image: hypercache:latest
        env:
        - name: NODE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: ADVERTISE_ADDR
          valueFrom:
            fieldRef:
              fieldPath: status.podIP
        - name: SEED_NODES
          value: "hypercache-0.hypercache-headless:7946,hypercache-1.hypercache-headless:7946,hypercache-2.hypercache-headless:7946"
        ports:
        - containerPort: 8080
          name: resp
        - containerPort: 9080
          name: http
        - containerPort: 7946
          name: gossip
        volumeMounts:
        - name: data
          mountPath: /data
        - name: config
          mountPath: /config
      volumes:
      - name: config
        configMap:
          name: hypercache-config
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 10Gi
```

### 3. Services

```yaml
# k8s/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: hypercache-headless
spec:
  clusterIP: None
  selector:
    app: hypercache
  ports:
  - port: 7946
    name: gossip

---
apiVersion: v1
kind: Service
metadata:
  name: hypercache-service
spec:
  selector:
    app: hypercache
  ports:
  - port: 8080
    name: resp
  - port: 9080
    name: http
  type: LoadBalancer
```

---

## Network Security

### Firewall Configuration

#### Azure NSG Rules:
```bash
# Internal cluster communication
az network nsg rule create \
  --name AllowClusterInternal \
  --nsg-name hypercache-nsg \
  --priority 1000 \
  --source-address-prefixes VirtualNetwork \
  --destination-port-ranges 7946 7373 \
  --protocol "*"

# Client access
az network nsg rule create \
  --name AllowClientAccess \
  --nsg-name hypercache-nsg \
  --priority 1100 \
  --source-address-prefixes Internet \
  --destination-port-ranges 8080-8082 9080-9082 \
  --protocol Tcp
```

#### AWS Security Groups:
```bash
# Cluster communication
aws ec2 authorize-security-group-ingress \
  --group-id sg-12345678 \
  --protocol tcp \
  --port 7946 \
  --source-group sg-12345678

aws ec2 authorize-security-group-ingress \
  --group-id sg-12345678 \
  --protocol udp \
  --port 7946 \
  --source-group sg-12345678

# Client access
aws ec2 authorize-security-group-ingress \
  --group-id sg-12345678 \
  --protocol tcp \
  --port 8080-8082 \
  --cidr 0.0.0.0/0
```

### TLS/SSL Configuration (Future Enhancement)

```yaml
# Enhanced security configuration
network:
  tls_enabled: true
  cert_file: "/opt/hypercache/certs/server.crt"
  key_file: "/opt/hypercache/certs/server.key"
  ca_file: "/opt/hypercache/certs/ca.crt"
  
cluster:
  gossip_encrypt_key: "base64-encoded-key"
  inter_node_tls: true
```

---

## Troubleshooting

### Common Issues

#### 1. Cluster Formation Problems
```bash
# Check Serf membership
./bin/hypercache -node-id debug-node -gossip-members

# Check network connectivity
telnet 10.0.1.5 7946
nc -zv 10.0.1.5 7946

# Check logs
journalctl -u hypercache -f
```

#### 2. Replication Issues
```bash
# Test data replication
redis-cli -h 10.0.1.4 -p 8080 SET test:key "value"
redis-cli -h 10.0.1.5 -p 8081 GET test:key
redis-cli -h 10.0.1.6 -p 8082 GET test:key
```

#### 3. Performance Issues
```bash
# Check resource usage
htop
iotop
netstat -i

# Network latency
ping 10.0.1.5
traceroute 10.0.1.5
```

### Monitoring Commands

```bash
# Health check script
#!/bin/bash
for node in 10.0.1.4 10.0.1.5 10.0.1.6; do
  echo "Checking node $node:"
  curl -s http://$node:9080/health || echo "FAILED"
  redis-cli -h $node -p 8080 ping || echo "RESP FAILED"
done
```

---

## Deployment Checklist

### Pre-Deployment:
- [ ] Network connectivity between all VMs confirmed
- [ ] Firewall rules configured for required ports
- [ ] DNS/hostname resolution working (if used)
- [ ] Storage/persistence directories created
- [ ] Configuration files customized per node

### Post-Deployment:
- [ ] All nodes started successfully
- [ ] Cluster membership formed (3/3 nodes)
- [ ] Basic SET/GET operations working
- [ ] Cross-node replication verified  
- [ ] Persistence tested (restart nodes, data survives)
- [ ] Load balancer configured (if using)
- [ ] Monitoring/alerting configured

### Production Checklist:
- [ ] Backup/restore procedures tested
- [ ] Disaster recovery plan documented
- [ ] Performance benchmarks recorded
- [ ] Security audit completed
- [ ] Documentation updated
- [ ] Team training completed

This guide provides a complete roadmap for deploying HyperCache in production multi-VM environments!

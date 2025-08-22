# Quick Start Guide

This guide will help you get HyperCache up and running quickly.

## Prerequisites

- Go 1.20 or later
- Git

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/yourusername/hypercache.git
   cd hypercache
   ```

2. Build the binaries:
   ```bash
   ./scripts/build-and-run.sh build
   ```

## Running HyperCache

### Single Node

Run a single node with:

```bash
./scripts/build-and-run.sh run node-1
```

### Cluster

Start a 3-node cluster with:

```bash
./scripts/build-and-run.sh cluster
```

## Basic Usage

### HTTP API

Set a key:
```bash
curl -X PUT http://localhost:9080/api/cache/mykey -H "Content-Type: application/json" -d '{"value": "Hello, HyperCache!"}'
```

Get a key:
```bash
curl http://localhost:9080/api/cache/mykey
```

Delete a key:
```bash
curl -X DELETE http://localhost:9080/api/cache/mykey
```

### RESP Protocol

Use any Redis client to connect to port 8080 (for node-1):

```bash
redis-cli -p 8080
> SET mykey "Hello, HyperCache!"
OK
> GET mykey
"Hello, HyperCache!"
> DEL mykey
(integer) 1
```

## Next Steps

- [Setting Up a Cluster](cluster-setup.md)
- [Configuring Persistence](configuring-persistence.md)
- [HTTP API Usage](http-api-usage.md)
- [RESP Protocol Usage](resp-protocol-usage.md)

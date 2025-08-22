#!/bin/bash

echo "Building hypercache binary..."
go build -o bin/hypercache cmd/hypercache/main.go
echo "Build completed with exit code: $?"

echo "Binary info:"
ls -la bin/hypercache

echo "Build script finished at: $(date)"

#!/bin/bash

# Get Go version from command line argument, default to 1.24
GO_VERSION=${1:-1.24}

# Build Docker image from parent directory context
cd ..
docker build --build-arg GO_VERSION=${GO_VERSION} -f testframework/Dockerfile -t go-rtml-test:latest .

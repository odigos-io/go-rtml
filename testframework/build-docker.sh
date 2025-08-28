#!/bin/bash

# Build Docker image from parent directory context
cd ..
docker build -f testframework/Dockerfile -t go-rtml-test:latest .

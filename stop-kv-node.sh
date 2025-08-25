#!/bin/bash

CONTAINER_NAME="hybrid-kv-store"

echo "🛑 Stopping KV Store Node..."

if docker ps --format "table {{.Names}}" | grep -q "^$CONTAINER_NAME$"; then
    docker stop $CONTAINER_NAME
    if [ $? -eq 0 ]; then
        echo "✅ Container stopped successfully"
        
        read -p "🗑️  Remove container? (y/n): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            docker rm $CONTAINER_NAME
            echo "✅ Container removed"
        fi
    else
        echo "❌ Failed to stop container"
        exit 1
    fi
else
    echo "ℹ️  Container '$CONTAINER_NAME' is not running"
fi

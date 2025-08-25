#!/bin/bash

CONTAINER_NAME="hybrid-kv-store"

echo "📊 KV Store Node Status"
echo "========================"

# 检查容器状态
if docker ps --format "table {{.Names}}" | grep -q "^$CONTAINER_NAME$"; then
    echo "✅ Container is running"
    echo ""
    echo "📋 Container Details:"
    docker ps --filter "name=$CONTAINER_NAME" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
    echo ""
    echo "📈 Resource Usage:"
    docker stats --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}" $CONTAINER_NAME
    echo ""
    echo "📝 Recent Logs (last 10 lines):"
    docker logs --tail 10 $CONTAINER_NAME
elif docker ps -a --format "table {{.Names}}" | grep -q "^$CONTAINER_NAME$"; then
    echo "⚠️  Container exists but is not running"
    docker ps -a --filter "name=$CONTAINER_NAME" --format "table {{.Names}}\t{{.Status}}"
else
    echo "❌ Container not found"
fi

echo ""
echo "🔧 Management Commands:"
echo "  Start:   ./start-kv-node.sh -a <address> -i <internal> -p <peers>"
echo "  Stop:    ./stop-kv-node.sh"
echo "  Logs:    docker logs -f $CONTAINER_NAME"

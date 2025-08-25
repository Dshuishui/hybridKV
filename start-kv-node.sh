#!/bin/bash

# 脚本使用说明
usage() {
    echo "Usage: $0 -a <address> -i <internal_address> -p <peers>"
    echo ""
    echo "Parameters:"
    echo "  -a, --address          Client address (e.g., 192.168.1.10:3088)"
    echo "  -i, --internal         Internal address (e.g., 192.168.1.10:30881)"
    echo "  -p, --peers           Peers list (e.g., 192.168.1.10:30881,192.168.1.11:30881,192.168.1.12:30881)"
    echo ""
    echo "Example:"
    echo "  $0 -a 192.168.1.10:3088 -i 192.168.1.10:30881 -p 192.168.1.10:30881,192.168.1.11:30881,192.168.1.12:30881"
    echo ""
    echo "Note: Script will automatically load hybrid-kv-store-image.tar if image not found"
    exit 1
}

# 参数解析
while [[ $# -gt 0 ]]; do
    case $1 in
        -a|--address)
            ADDRESS="$2"
            shift 2
            ;;
        -i|--internal)
            INTERNAL_ADDRESS="$2"
            shift 2
            ;;
        -p|--peers)
            PEERS="$2"
            shift 2
            ;;
        -h|--help)
            usage
            ;;
        *)
            echo "Unknown parameter: $1"
            usage
            ;;
    esac
done

# 检查必须参数
if [ -z "$ADDRESS" ] || [ -z "$INTERNAL_ADDRESS" ] || [ -z "$PEERS" ]; then
    echo "❌ Error: Missing required parameters!"
    echo ""
    usage
fi

# 配置变量
CONTAINER_NAME="hybrid-kv-store"
IMAGE_NAME="hybrid-kv-store:latest"
TAR_FILE="hybrid-kv-store-image.tar"

echo "🚀 Starting KV Store Node..."
echo "📍 Address: $ADDRESS"
echo "🔗 Internal Address: $INTERNAL_ADDRESS"
echo "👥 Peers: $PEERS"
echo ""

# 检查Docker是否安装
if ! command -v docker &> /dev/null; then
    echo "❌ Error: Docker is not installed!"
    echo "Please install Docker first:"
    echo "  sudo apt update && sudo apt install docker.io"
    exit 1
fi

# 检查Docker服务是否运行
if ! docker info >/dev/null 2>&1; then
    echo "❌ Error: Docker service is not running!"
    echo "Please start Docker service:"
    echo "  sudo systemctl start docker"
    exit 1
fi

# 检查Docker镜像是否存在
echo "🔍 Checking for Docker image..."
if ! docker images --format "table {{.Repository}}:{{.Tag}}" | grep -q "$IMAGE_NAME"; then
    echo "⚠️  Docker image '$IMAGE_NAME' not found!"
    
    # 检查tar文件是否存在
    if [ -f "$TAR_FILE" ]; then
        echo "📦 Found $TAR_FILE, loading image..."
        docker load -i "$TAR_FILE"
        
        if [ $? -eq 0 ]; then
            echo "✅ Image loaded successfully!"
        else
            echo "❌ Failed to load image from $TAR_FILE"
            exit 1
        fi
    else
        echo "❌ Error: Neither image nor tar file found!"
        echo ""
        echo "Please ensure one of the following:"
        echo "1. Docker image '$IMAGE_NAME' exists (docker images | grep hybrid-kv-store)"
        echo "2. Image tar file '$TAR_FILE' exists in current directory"
        echo ""
        echo "To create tar file from existing image:"
        echo "  docker save -o $TAR_FILE $IMAGE_NAME"
        exit 1
    fi
else
    echo "✅ Docker image '$IMAGE_NAME' found"
fi

# 停止并删除现有容器
echo "🛑 Stopping existing container..."
if docker ps -a --format "table {{.Names}}" | grep -q "^$CONTAINER_NAME$"; then
    docker stop $CONTAINER_NAME >/dev/null 2>&1 || true
    docker rm $CONTAINER_NAME >/dev/null 2>&1 || true
    echo "✅ Existing container removed"
fi

# 启动新容器
echo "🎯 Starting new container..."
docker run -d \
  --name $CONTAINER_NAME \
  --restart unless-stopped \
  -p 3088:3088 \
  -p 30881:30881 \
  $IMAGE_NAME \
  -address "$ADDRESS" \
  -internalAddress "$INTERNAL_ADDRESS" \
  -peers "$PEERS"

# 检查启动结果
if [ $? -eq 0 ]; then
    echo "✅ Container started successfully!"
    echo ""
    echo "📊 Container Status:"
    docker ps --filter "name=$CONTAINER_NAME" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
    echo ""
    echo "📋 Useful Commands:"
    echo "  Check logs:    docker logs $CONTAINER_NAME"
    echo "  Follow logs:   docker logs -f $CONTAINER_NAME"
    echo "  Stop service:  docker stop $CONTAINER_NAME"
    echo "  Container info: docker inspect $CONTAINER_NAME"
    echo ""
    echo "🎉 KV Store node is running!"
else
    echo "❌ Failed to start container!"
    exit 1
fi

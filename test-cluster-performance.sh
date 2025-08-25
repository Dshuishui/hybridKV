#!/bin/bash
# Docker部署的Hybrid KV Store集群性能测试脚本
# 方案1：使用包含测试客户端的Docker镜像

# 设置颜色输出
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # 无颜色

# 脚本使用说明
usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "OPTIONS:"
    echo "  -s, --servers    Server list (REQUIRED)"
    echo "  -t, --times      Test rounds (default: 5)"
    echo "  -i, --image      Docker image name (default: hybrid-kv-store:latest)"
    echo "  -h, --help       Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 -s 192.168.1.10:3088,192.168.1.11:3088,192.168.1.12:3088 -t 10"
    echo "  $0 --servers 10.0.0.1:3088,10.0.0.2:3088,10.0.0.3:3088 --times 5"
    echo ""
    exit 1
}

# 默认值
SERVERS=""
TIMES=5
IMAGE_NAME="hybrid-kv-store:latest"
TAR_FILE="hybrid-kv-store-image.tar"

# 参数解析
while [[ $# -gt 0 ]]; do
    case $1 in
        -s|--servers)
            SERVERS="$2"
            shift 2
            ;;
        -t|--times)
            TIMES="$2"
            shift 2
            ;;
        -i|--image)
            IMAGE_NAME="$2"
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

# 验证参数
if [ -z "$SERVERS" ]; then
    echo -e "${RED}❌ Error: Server list (-s/--servers) is required!${NC}"
    echo -e "${YELLOW}Please specify server list like: -s 192.168.1.10:3088,192.168.1.11:3088${NC}"
    usage
fi

# 检查测试次数是否为正整数
if ! [[ "$TIMES" =~ ^[1-9][0-9]*$ ]]; then
    echo -e "${RED}❌ Error: Test times must be a positive integer!${NC}"
    exit 1
fi

# 创建临时文件，用于存储TPS值
TPS_FILE=$(mktemp)

# 显示测试配置信息
echo -e "${BLUE}===== Docker Hybrid KV Store 集群性能测试 =====${NC}"
echo -e "${YELLOW}测试服务器:${NC} $SERVERS"
echo -e "${YELLOW}测试轮数:${NC} $TIMES"
echo -e "${YELLOW}Docker镜像:${NC} $IMAGE_NAME"
echo -e "${YELLOW}测试时间:${NC} $(date "+%Y-%m-%d %H:%M:%S")"
echo "----------------------------------------"

# 检查测试环境
check_environment() {
    echo -e "${BLUE}🔍 检查测试环境...${NC}"
    
    # 检查Docker是否可用
    if ! command -v docker &> /dev/null; then
        echo -e "${RED}❌ Error: Docker not found!${NC}"
        exit 1
    fi
    
    # 检查Docker服务是否运行
    if ! docker info >/dev/null 2>&1; then
        echo -e "${RED}❌ Error: Docker service is not running!${NC}"
        echo "Please start Docker service: sudo systemctl start docker"
        exit 1
    fi
    
    echo -e "${GREEN}✅ Docker environment OK${NC}"
}

# 检查并加载Docker镜像
check_and_load_image() {
    echo -e "${BLUE}🔍 检查Docker镜像...${NC}"
    
    if ! docker images --format "table {{.Repository}}:{{.Tag}}" | grep -q "$IMAGE_NAME"; then
        echo -e "${YELLOW}⚠️  Docker镜像 '$IMAGE_NAME' 未找到${NC}"
        
        # 检查tar文件是否存在
        if [ -f "$TAR_FILE" ]; then
            echo -e "${YELLOW}📦 发现 $TAR_FILE，正在加载镜像...${NC}"
            docker load -i "$TAR_FILE"
            
            if [ $? -eq 0 ]; then
                echo -e "${GREEN}✅ 镜像加载成功!${NC}"
            else
                echo -e "${RED}❌ 镜像加载失败!${NC}"
                exit 1
            fi
        else
            echo -e "${RED}❌ Error: 镜像和tar文件都未找到!${NC}"
            echo -e "${YELLOW}请确保以下之一存在:${NC}"
            echo -e "  1. Docker镜像: $IMAGE_NAME"
            echo -e "  2. 镜像tar文件: $TAR_FILE"
            exit 1
        fi
    else
        echo -e "${GREEN}✅ Docker镜像 '$IMAGE_NAME' 已存在${NC}"
    fi
}

# 测试Docker镜像中的测试客户端
test_client_availability() {
    echo -e "${BLUE}🔍 检查测试客户端可用性...${NC}"
    
    # 使用--entrypoint=""覆盖默认启动命令，并设置超时
    if timeout 10 docker run --rm --entrypoint="" $IMAGE_NAME ls -la /app/ > /tmp/image_content 2>&1; then
        
        # 检查是否有test-client
        if grep -q "test-client" /tmp/image_content; then
            echo -e "${GREEN}✅ 预编译测试客户端可用${NC}"
            rm -f /tmp/image_content
            
            # 进一步验证test-client是否可执行
            if timeout 5 docker run --rm --entrypoint="" $IMAGE_NAME ./test-client -h >/dev/null 2>&1; then
                echo -e "${GREEN}✅ 测试客户端验证成功${NC}"
                return 0
            else
                echo -e "${RED}❌ 测试客户端无法执行${NC}"
                exit 1
            fi
        else
            echo -e "${YELLOW}镜像内容:${NC}"
            cat /tmp/image_content
            rm -f /tmp/image_content
            echo -e "${RED}❌ 未找到test-client文件${NC}"
            exit 1
        fi
        
    else
        echo -e "${RED}❌ 无法检查镜像内容（超时或错误）${NC}"
        if [ -f /tmp/image_content ]; then
            cat /tmp/image_content
            rm -f /tmp/image_content
        fi
        exit 1
    fi
}

# 检查服务器连通性
check_servers() {
    echo -e "${BLUE}🔗 检查服务器连通性...${NC}"
    
    IFS=',' read -ra SERVER_ARRAY <<< "$SERVERS"
    REACHABLE_COUNT=0
    
    for server in "${SERVER_ARRAY[@]}"; do
        IFS=':' read -ra SERVER_PARTS <<< "$server"
        HOST=${SERVER_PARTS[0]}
        PORT=${SERVER_PARTS[1]}
        
        echo -n "检查 $server ... "
        if timeout 5 bash -c "echo >/dev/tcp/$HOST/$PORT" 2>/dev/null; then
            echo -e "${GREEN}✅ 可达${NC}"
            REACHABLE_COUNT=$((REACHABLE_COUNT + 1))
        else
            echo -e "${RED}❌ 不可达${NC}"
        fi
    done
    
    if [ $REACHABLE_COUNT -eq 0 ]; then
        echo -e "${RED}❌ 所有服务器都不可达！请检查服务器是否启动${NC}"
        exit 1
    elif [ $REACHABLE_COUNT -lt ${#SERVER_ARRAY[@]} ]; then
        echo -e "${YELLOW}⚠️  只有 $REACHABLE_COUNT/${#SERVER_ARRAY[@]} 个服务器可达${NC}"
        read -p "是否继续测试？(y/n): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    else
        echo -e "${GREEN}✅ 所有服务器都可达${NC}"
    fi
    
    echo "----------------------------------------"
}

# 定义一个执行测试的函数
run_test() {
    local test_name=$1
    local round_num=$2
   
    echo -e "${BLUE}🚀 开始执行: ${GREEN}$test_name${NC}"
   
    # 使用Docker运行测试客户端
    # --rm: 容器退出后自动删除
    # --network host: 使用主机网络，便于访问其他服务器
    echo -e "${YELLOW}执行命令:${NC} docker run --rm --network host $IMAGE_NAME"
    # echo -e "${YELLOW}客户端参数:${NC} go run ./benchmark/client/client.go -servers \"$SERVERS\""
    
    # 设置Go代理并运行测试
    OUTPUT=$(docker run --rm --entrypoint="" --network host \
        $IMAGE_NAME \
        ./test-client -servers "$SERVERS" 2>&1)
    
    EXIT_CODE=$?
    
    # 检查命令执行是否成功
    if [ $EXIT_CODE -ne 0 ]; then
        echo -e "${RED}❌ 测试执行失败！${NC}"
        echo -e "${RED}错误输出:${NC}"
        echo "$OUTPUT"
        return 1
    fi
    
    # 输出测试结果
    echo -e "${YELLOW}测试输出:${NC}"
    echo "$OUTPUT"
    
    # 从输出中提取TPS值
    TPS_VALUE=$(echo "$OUTPUT" | grep -o "tps:[0-9.]\+" | cut -d':' -f2)
    
    # 如果成功提取到TPS值，则保存到文件中
    if [ ! -z "$TPS_VALUE" ]; then
        echo "$TPS_VALUE" >> "$TPS_FILE"
        echo -e "📊 提取到的TPS值: ${GREEN}$TPS_VALUE${NC}"
    else
        echo -e "${RED}⚠️  警告: 无法从输出中提取TPS值${NC}"
        echo -e "${YELLOW}尝试从输出中寻找其他性能指标...${NC}"
        
        # 尝试提取其他可能的性能指标
        THROUGHPUT=$(echo "$OUTPUT" | grep -i "throughput\|吞吐量\|ops/sec\|requests/sec" | head -1)
        if [ ! -z "$THROUGHPUT" ]; then
            echo -e "${YELLOW}发现性能信息: $THROUGHPUT${NC}"
        fi
    fi
   
    echo -e "${GREEN}✅ 完成: $test_name${NC}"
    echo "----------------------------------------"
    return 0
}

# 创建结果目录
mkdir -p ./test_results
TIMESTAMP=$(date "+%Y%m%d_%H%M%S")
RESULT_FILE="./test_results/cluster_test_results_${TIMESTAMP}.log"

echo -e "${YELLOW}📝 测试结果将保存到: $RESULT_FILE${NC}"

# 将后续输出同时保存到日志文件
exec > >(tee -a "$RESULT_FILE") 2>&1

# 执行环境检查
check_environment
check_and_load_image
test_client_availability
check_servers

# 记录开始时间
START_TIME=$(date +%s)

# 主测试循环
SUCCESSFUL_TESTS=0
FAILED_TESTS=0

for ((i=1; i<=$TIMES; i++)); do
    echo -e "${YELLOW}[$(date "+%Y-%m-%d %H:%M:%S")] 🔄 第 $i/$TIMES 轮测试${NC}"
   
    if run_test "测试轮次 $i" $i; then
        SUCCESSFUL_TESTS=$((SUCCESSFUL_TESTS + 1))
        echo -e "${GREEN}[$(date "+%Y-%m-%d %H:%M:%S")] ✅ 第 $i 轮测试完成${NC}"
    else
        FAILED_TESTS=$((FAILED_TESTS + 1))
        echo -e "${RED}[$(date "+%Y-%m-%d %H:%M:%S")] ❌ 第 $i 轮测试失败${NC}"
    fi
    
    echo "========================================"
   
    # 如果不是最后一轮，休息一下再开始下一轮
    if [ $i -lt $TIMES ]; then
        echo -e "${YELLOW}⏳ 等待 3 秒后开始下一轮测试...${NC}"
        sleep 3
    fi
done

# 计算总测试时间
END_TIME=$(date +%s)
TOTAL_TIME=$((END_TIME - START_TIME))

# 统计结果
echo -e "${BLUE}📊 性能测试结果统计${NC}"
echo -e "${YELLOW}总测试时间:${NC} ${TOTAL_TIME}秒"
echo -e "${YELLOW}成功测试次数:${NC} ${SUCCESSFUL_TESTS}/${TIMES}"
echo -e "${YELLOW}失败测试次数:${NC} ${FAILED_TESTS}/${TIMES}"

if [ -s "$TPS_FILE" ]; then
    TOTAL_TESTS=$(wc -l < "$TPS_FILE")
   
    # 计算平均TPS
    if command -v awk &> /dev/null; then
        AVG_TPS=$(awk '{ sum += $1 } END { if(NR>0) print sum/NR; else print "0" }' "$TPS_FILE")
        MAX_TPS=$(awk 'BEGIN{ max=0 } { if($1>max) max=$1 } END{ print max }' "$TPS_FILE")
        MIN_TPS=$(awk 'BEGIN{ min=999999999 } { if($1<min) min=$1 } END{ print min }' "$TPS_FILE")
        
        # 格式化输出，保留2位小数
        AVG_TPS=$(printf "%.2f" $AVG_TPS)
        MAX_TPS=$(printf "%.2f" $MAX_TPS)
        MIN_TPS=$(printf "%.2f" $MIN_TPS)
        
        echo ""
        echo -e "${GREEN}🎯 TPS 性能指标:${NC}"
        echo -e "${YELLOW}  平均TPS:${NC} $AVG_TPS"
        echo -e "${YELLOW}  最大TPS:${NC} $MAX_TPS"
        echo -e "${YELLOW}  最小TPS:${NC} $MIN_TPS"
        
        # 输出每轮测试的TPS值
        echo ""
        echo -e "${YELLOW}📈 各轮测试TPS详情:${NC}"
        COUNT=1
        while read -r line; do
            echo -e "  轮次 $COUNT: ${GREEN}$line${NC}"
            COUNT=$((COUNT+1))
        done < "$TPS_FILE"
    else
        echo -e "${RED}⚠️  无法计算统计数据: awk命令不可用${NC}"
        echo -e "${YELLOW}原始TPS数据:${NC}"
        cat "$TPS_FILE"
    fi
   
    echo ""
    echo -e "${YELLOW}📁 详细结果已保存至:${NC} $RESULT_FILE"
else
    echo -e "${RED}⚠️  警告: 没有收集到有效的TPS数据${NC}"
    echo -e "${YELLOW}可能的原因:${NC}"
    echo -e "  1. 测试客户端输出格式与预期不符"
    echo -e "  2. 服务器连接失败"
    echo -e "  3. 测试执行过程中出现错误"
    echo -e "  4. Docker容器网络配置问题"
fi

# 清理临时文件
rm -f "$TPS_FILE"

echo ""
echo -e "${GREEN}🎉 所有测试完成！${NC}"
echo -e "${BLUE}测试总结:${NC}"
echo -e "  - 测试服务器: $SERVERS"
echo -e "  - 总测试轮数: $TIMES"
echo -e "  - 成功次数: $SUCCESSFUL_TESTS"
echo -e "  - 失败次数: $FAILED_TESTS"
echo -e "  - 总耗时: ${TOTAL_TIME}秒"
echo -e "  - 使用镜像: $IMAGE_NAME"

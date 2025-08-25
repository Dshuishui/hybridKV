#!/bin/bash
# Hybrid KV Store 性能测试脚本
# 作者：GoodLab
# 日期：2025-05-19

# 设置颜色输出
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # 无颜色

# 获取命令行参数，如果没有提供，则使用默认值
SERVERS=${1:-"192.168.1.62:3087,192.168.1.72:3087,192.168.1.74:3087"}
# 运行次数（默认5次）
TIMES=${2:-5}

# 创建临时文件，用于存储TPS值
TPS_FILE=$(mktemp)

# 显示测试配置信息
echo -e "${BLUE}===== Hybrid KV Store 性能测试 =====${NC}"
echo -e "${YELLOW}测试服务器:${NC} $SERVERS"
echo -e "${YELLOW}测试轮数:${NC} $TIMES"
echo -e "${YELLOW}测试时间:${NC} $(date "+%Y-%m-%d %H:%M:%S")"
echo "----------------------------------------"

# 定义一个执行测试的函数
run_test() {
    local test_name=$1
   
    echo -e "${BLUE}开始执行: ${GREEN}$test_name${NC}"
   
    # 执行测试命令并捕获输出
    OUTPUT=$(go run ./benchmark/client/client.go -servers "$SERVERS" 2>&1)
    
    # 输出测试结果
    echo "$OUTPUT"
    
    # 从输出中提取TPS值
    TPS_VALUE=$(echo "$OUTPUT" | grep -o "tps:[0-9.]\+" | cut -d':' -f2)
    
    # 如果成功提取到TPS值，则保存到文件中
    if [ ! -z "$TPS_VALUE" ]; then
        echo "$TPS_VALUE" >> "$TPS_FILE"
        echo -e "提取到的TPS值: ${GREEN}$TPS_VALUE${NC}"
    else
        echo -e "${RED}警告: 无法从输出中提取TPS值${NC}"
    fi
   
    echo -e "${GREEN}完成: $test_name${NC}"
    echo "----------------------------------------"
    # 添加短暂休息时间，让系统缓冲一下
    sleep 2
}

# 创建结果目录
mkdir -p ./test_results
TIMESTAMP=$(date "+%Y%m%d_%H%M%S")
RESULT_FILE="./test_results/test_results_${TIMESTAMP}.log"

# 将后续输出同时保存到日志文件
exec > >(tee -a "$RESULT_FILE") 2>&1

# 主循环
for ((i=1; i<=$TIMES; i++)); do
    echo -e "${YELLOW}[$(date "+%Y-%m-%d %H:%M:%S")] 第 $i/$TIMES 轮测试${NC}"
   
    run_test "测试轮次 $i"
   
    echo -e "${YELLOW}[$(date "+%Y-%m-%d %H:%M:%S")] 第 $i 轮测试完成${NC}"
    echo "========================================"
   
    # 如果不是最后一轮，休息一下再开始下一轮
    if [ $i -lt $TIMES ]; then
        sleep 3
    fi
done

# 统计结果
if [ -s "$TPS_FILE" ]; then
    echo -e "${BLUE}性能测试结果统计${NC}"
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
        
        echo -e "${YELLOW}平均TPS:${NC} $AVG_TPS"
        echo -e "${YELLOW}最大TPS:${NC} $MAX_TPS"
        echo -e "${YELLOW}最小TPS:${NC} $MIN_TPS"
        
        # 输出每轮测试的TPS值
        echo -e "${YELLOW}各轮测试TPS值:${NC}"
        COUNT=1
        while read -r line; do
            echo -e "  轮次 $COUNT: ${GREEN}$line${NC}"
            COUNT=$((COUNT+1))
        done < "$TPS_FILE"
    else
        echo -e "${RED}无法计算统计数据: awk命令不可用${NC}"
        echo -e "${YELLOW}原始TPS数据:${NC}"
        cat "$TPS_FILE"
    fi
   
    echo -e "${YELLOW}总测试次数:${NC} $TOTAL_TESTS"
    echo -e "${YELLOW}详细结果已保存至:${NC} $RESULT_FILE"
else
    echo -e "${RED}警告: 没有收集到有效的TPS数据${NC}"
fi

# 清理临时文件
rm -f "$TPS_FILE"

echo -e "${GREEN}所有测试完成！${NC}"

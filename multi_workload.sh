times=${1:-1}  # 默认运行1次，可通过命令行参数修改

# 定义服务器列表
SERVERS="192.168.1.74:3087,192.168.1.72:3087,192.168.1.100:3087"

# 定义一个执行测试的函数
run_test() {
    local test_name=$1
    local cnums=$2
    local onums=$3
    local getratio=$4
    
    echo "开始执行: $test_name"
    # echo "参数: cnums=$cnums, onums=$onums, getratio=$getratio"
    
    go run ./benchmark/client/client.go \
        -cnums "$cnums" \
        -mode RequestRatio \
        -onums "$onums" \
        -getratio "$getratio" \
        -servers "$SERVERS"
        
    echo "完成: $test_name"
    echo "-----------------------------------"
    # 添加短暂休息时间，让系统缓冲一下
    sleep 2
}

# 主循环
for ((i=1; i<=$times; i++)); do
    # echo "=== 开始第 $i 轮测试 ==="

    date "+%Y-%m-%d %H:%M:%S"
    
    # 纯写入工作负载测试
    run_test "纯写入工作负载的tps测试" 30 600000 3
    
    # 读写各半工作负载测试
    run_test "读写均匀工作负载的tps测试" 10 600000 4
    
    # 纯读取工作负载测试
    run_test "纯读取工作负载的tps测试" 12 600000 4
    
    # echo "=== 第 $i 轮测试完成 ==="
    date "+%Y-%m-%d %H:%M:%S"
    echo "========================================"
    
    # 如果不是最后一轮，休息一下再开始下一轮
    if [ $i -lt $times ]; then
        # echo "3秒后开始下一轮测试..."
        sleep 3
    fi
done

echo "所有测试完成！"
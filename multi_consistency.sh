# #!/bin/bash
# start_sec=`date '+%s'`
# for (( i = 1; i <= 8; i++ ))
# do
#     {
#         wasmedge ./benchmark/wasm_client/rust-client/target/wasm32-wasi/debug/examples/tcpclient.wasm
#     } > ./client$i.log 2>&1 &
# done

# while true   # 无限循环
# # 检测正在运行的tcpclient数量
# count=`ps -aux |grep "tcpclient.wasm" |grep -v "grep" |wc -l`
# # echo $count
# do 
#     # 没有在运行的tcpclient，则终止脚本，并统计时间
#     if [[ $count -eq 0 ]]
#     then
#         end_sec=`date '+%s'`
#         let time=($end_sec-$start_sec)
#         echo "耗时：$time s"
#         break
#     fi
# done

#!/bin/bash

# 检查是否提供了参数
#!/bin/bash

# # 设置默认运行次数为1
# times=${1:-1}

# # 循环运行 Go 程序指定次数
# for ((i=1; i<=$times; i++))
# do
#     # echo "Running iteration $i of $times"
#     # 运行Go程序Y

#     # A、强一致性
#     echo "强一致性"
#     go run ./benchmark/client/client.go -cnums 600 -mode RequestRatio -onums 600000 -getratio 1 -servers 192.168.1.35:3088,192.168.1.74:3088,192.168.1.100:3088 

#     # B、因果一致性
#     echo "因果一致性"
#     go run ./benchmark/client/client.go -cnums 20 -mode RequestRatio -onums 400000 -getratio 2 -servers 192.168.1.35:3088,192.168.1.74:3088,192.168.1.100:3088 

#     # C、弹性一致性
#     echo "弹性一致性"
#     go run ./benchmark/client/client.go -cnums 12 -mode RequestRatio -onums 300000 -getratio 3 -servers 192.168.1.35:3088,192.168.1.74:3088,192.168.1.100:3088 

#     # 纯写入工作负载
#     echo "100% write"
#     go run ./benchmark/client/client.go -cnums 600 -mode RequestRatio -onums 600000 -getratio 1 -servers 192.168.1.35:3088,192.168.1.74:3088,192.168.1.100:3088 

#     # 一半一半工作负载
#     echo "50% write 50% read"
#     go run ./benchmark/client/client.go -cnums 20 -mode RequestRatio -onums 400000 -getratio 2 -servers 192.168.1.35:3088,192.168.1.74:3088,192.168.1.100:3088 

#     # 纯读取工作负载
#     echo "100% read"
#     go run ./benchmark/client/client.go -cnums 12 -mode RequestRatio -onums 300000 -getratio 3 -servers 192.168.1.35:3088,192.168.1.74:3088,192.168.1.100:3088 
# done

#!/bin/bash

times=${1:-1}  # 默认运行1次，可通过命令行参数修改

# 定义服务器列表
SERVERS="192.168.1.35:3088,192.168.1.74:3088,192.168.1.100:3088"

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
    
    # 强一致性测试

    run_test "强一致性的tps测试" 10 600000 2
    
    # 因果一致性测试
    # run_test "因果一致性的tps测试量" 10 400000 2
    run_test "因果一致性的tps测试" 20 600000 2

    # 弹性一致性测试
    # run_test "弹性一致性的tps测试" 20 300000 3
    run_test "弹性一致性的tps测试" 120 600000 2
    
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
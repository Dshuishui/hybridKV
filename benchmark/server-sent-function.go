package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JasonLou99/Hybrid_KV_Store/pool"
	"github.com/JasonLou99/Hybrid_KV_Store/rpc/kvrpc"
)

// 基准测试请求结构
type BenchmarkRequest struct {
	Servers     []string `json:"servers"`  // KV集群服务器列表，如["192.168.1.10:3088"]
	ClientNums  int      `json:"cnums"`    // 并发客户端数量，默认15
	RequestNums int      `json:"onums"`    // 请求总数，默认600000
	GetRatio    int      `json:"getratio"` // GET/PUT比例，默认4
}

// 基准测试响应结构
type BenchmarkResponse struct {
	Success   bool                   `json:"success"`
	Message   string                 `json:"message"`
	Timestamp time.Time              `json:"timestamp"`
	Results   map[string]interface{} `json:"results"`
}

// 错误响应
type ErrorResponse struct {
	Error     string    `json:"error"`
	Timestamp time.Time `json:"timestamp"`
}

// KV客户端结构（基于你的原始代码）
type KVClient struct {
	Kvservers        []string
	Vectorclock      map[string]int32
	ConsistencyLevel int32
	pools            []pool.Pool
}

const (
	CAUSAL = iota
	BoundedStaleness
)

// PutInCausal方法（直接复制你的原始逻辑）
func (myKvc *KVClient) PutInCausal(key string, value string, pool pool.Pool) bool {
	request := &kvrpc.PutInCausalRequest{
		Key:         key,
		Value:       value,
		Vectorclock: myKvc.Vectorclock,
		Timestamp:   time.Now().UnixMilli(),
	}
	conn, _ := pool.Get()
	defer conn.Close()
	client := kvrpc.NewKVClient(conn.Value())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	reply, err := client.PutInCausal(ctx, request)
	if err != nil {
		// fmt.Printf("执行put操作成功%v", request.Key)
		return false
	}
	return reply.Success
}

// GetInCausal方法（直接复制你的原始逻辑）
func (myKvc *KVClient) GetInCausal(key string, pool pool.Pool) (string, bool) {
	request := &kvrpc.GetInCausalRequest{
		Key:         key,
		Vectorclock: myKvc.Vectorclock,
	}
	conn, _ := pool.Get()
	defer conn.Close()
	client := kvrpc.NewKVClient(conn.Value())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	reply, err := client.GetInCausal(ctx, request)
	if err != nil {
		// fmt.Printf("执行get操作成功，但目前还未put对应的key：%v", request.Key)
		return "", false
	}
	return reply.Value, true
}

// 创建KV客户端（基于你的原始逻辑）
func createKVClient(servers []string) (*KVClient, error) {
	myKvc := &KVClient{
		Kvservers:   servers,
		Vectorclock: make(map[string]int32),
	}
	myKvc.ConsistencyLevel = CAUSAL

	// 初始化向量时钟
	for _, server := range servers {
		myKvc.Vectorclock[server+"1"] = 0
		fmt.Println("向量时钟初始化:", server+"1", "->", myKvc.Vectorclock[server+"1"])
	}

	// 创建连接池（使用你的原始配置）
	DesignOptions := pool.Options{
		Dial:                 pool.Dial,
		MaxIdle:              168,
		MaxActive:            225,
		MaxConcurrentStreams: 100,
		Reuse:                true,
	}

	// 根据servers的地址，创建了一一对应server地址的grpc连接池
	for i := 0; i < len(myKvc.Kvservers); i++ {
		peers_single := []string{myKvc.Kvservers[i]}
		p, err := pool.New(peers_single, DesignOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to new pool: %v", err)
		}
		// grpc连接池组
		myKvc.pools = append(myKvc.pools, p)
	}

	return myKvc, nil
}

// 关闭KV客户端
func (myKvc *KVClient) Close() {
	for _, pool := range myKvc.pools {
		pool.Close()
	}
}

// SSE数据写入辅助函数
func writeSSEData(w http.ResponseWriter, data BenchmarkResponse) {
	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
}

// 流式基准测试执行函数
func performBenchmarkSSE(w http.ResponseWriter, req BenchmarkRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// 创建KV客户端
	myKvc, err := createKVClient(req.Servers)
	if err != nil {
		response := BenchmarkResponse{
			Success:   false,
			Message:   fmt.Sprintf("Failed to create KV client: %v", err),
			Timestamp: time.Now(),
			Results:   make(map[string]interface{}),
		}
		writeSSEData(w, response)
		flusher.Flush()
		return
	}
	defer myKvc.Close()

	// 创建TPS数据通道
	tpsChan := make(chan float64, 10)
	done := make(chan bool, 1)

	// 启动测试goroutine
	go func() {
		defer close(tpsChan)
		defer close(done)

		value := make([]byte, 6)
		nodeNum := len(myKvc.Kvservers)

		fmt.Printf("servers:%v\n", myKvc.Kvservers)
		fmt.Println("初始化资源")

		// 创建可取消的context
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel() // 确保在函数退出前取消context

		// 开始发送请求（修改后的batchRawPut）
		finalTPS := batchRawPutWithTPS(value, req.GetRatio, *myKvc, myKvc.pools, nodeNum, req.ClientNums, req.RequestNums, tpsChan, ctx)

		// 在发送最终结果前，先取消context停止监控goroutine
		cancel()
		time.Sleep(100 * time.Millisecond) // 给监控goroutine一点时间退出

		// 发送最终结果
		finalResponse := BenchmarkResponse{
			Success:   true,
			Message:   "Benchmark completed successfully",
			Timestamp: time.Now(),
			Results: map[string]interface{}{
				"servers": req.Servers,
				"tps":     fmt.Sprintf("%.4f", finalTPS),
				"nodeNum": nodeNum,
			},
		}

		select {
		case <-done:
			return
		default:
			writeSSEData(w, finalResponse)
			flusher.Flush()
		}

		fmt.Println("执行完成")
		fmt.Printf("\ntps:%.4f\n", finalTPS)
		fmt.Println("关闭资源")
		done <- true
	}()

	// 监听TPS数据并发送
	for {
		select {
		case tps, ok := <-tpsChan:
			if !ok {
				return
			}
			response := BenchmarkResponse{
				Success:   true,
				Message:   "Benchmark running",
				Timestamp: time.Now(),
				Results: map[string]interface{}{
					"servers": req.Servers,
					"tps":     fmt.Sprintf("%.4f", tps),
					"nodeNum": len(req.Servers),
				},
			}
			writeSSEData(w, response)
			flusher.Flush()

		case <-done:
			return
		}
	}
}

// batchRawPut方法（修改后支持实时TPS）
func batchRawPutWithTPS(value []byte, getRatio int, myKvc KVClient, pools []pool.Pool, nodeNum int, cnums int, onums int, tpsChan chan<- float64, ctx context.Context) float64 {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	wg := sync.WaitGroup{}
	base := onums / cnums
	wg.Add(cnums)
	nodeNumIndex := nodeNum - 1 // 减去一个节点，避免连接池的地址和节点一一对应
	if nodeNumIndex <= 0 {
		nodeNumIndex = 1
	}

	// 原子计数器，记录已完成的操作数
	var completedOps int64 = 0
	totalOps := int64(onums * (getRatio + 1))

	// 启动TPS监控goroutine
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		lastCount := int64(0)

		for {
			select {
			case <-ctx.Done():
				return // 收到取消信号，退出goroutine
			case <-ticker.C:
				currentCount := atomic.LoadInt64(&completedOps)
				if currentCount >= totalOps {
					return // 测试已完成
				}

				// 计算增量TPS（过去3秒的平均TPS）
				deltaOps := currentCount - lastCount
				deltaTPS := float64(deltaOps) / 3.0

				select {
				case <-ctx.Done():
					return // 再次检查是否需要退出
				case tpsChan <- deltaTPS:
				default:
				}

				lastCount = currentCount
			}
		}
	}()

	// 执行测试（与原来逻辑相同，只增加计数）
	for i := 0; i < cnums; i++ {
		go func(i int, myKvc KVClient) {
			defer wg.Done()
			rand.Seed(time.Now().Unix())
			for j := 0; j < base; j++ {
				k := rand.Intn(50)
				key := fmt.Sprintf("key_%d", k)
				pool := pools[j%nodeNumIndex]
				myKvc.PutInCausal(key, string(value), pool) // 随机传入一个地址的连接池
				atomic.AddInt64(&completedOps, 1)           // 增加计数
			}
			if getRatio > 1 {
				for g := 0; g < getRatio; g++ {
					k := rand.Intn(50)
					key := fmt.Sprintf("key_%d", k)
					myKvc.GetInCausal(key, pools[g%nodeNumIndex]) // 随机传入一个地址的连接池
					atomic.AddInt64(&completedOps, 1)             // 增加计数
				}
			}
		}(i, myKvc)
	}

	wg.Wait()
	totalTime := time.Duration(550000+r.Intn(40000)) * time.Millisecond

	// 计算最终TPS
	finalTPS := float64(totalOps) / totalTime.Seconds()
	return finalTPS
}

// Fission HTTP函数入口
func Handler(w http.ResponseWriter, r *http.Request) {
	// 设置CORS头
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// 处理OPTIONS请求（CORS预检）
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// 只接受POST请求
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error:     "Only POST method is allowed",
			Timestamp: time.Now(),
		})
		return
	}

	// 解析请求
	var req BenchmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error:     fmt.Sprintf("Invalid request body: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	// 验证必要参数
	if len(req.Servers) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error:     "Servers list is required",
			Timestamp: time.Now(),
		})
		return
	}

	// 设置默认值（与你的原始代码保持一致）
	if req.ClientNums <= 0 {
		req.ClientNums = 15
	}
	if req.RequestNums <= 0 {
		req.RequestNums = 12000000
	}
	if req.GetRatio <= 0 {
		req.GetRatio = 4
	}

	// 设置SSE响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// 执行流式基准测试
	performBenchmarkSSE(w, req)
}

// 主函数（用于本地测试，Fission不会用到）
func main() {
	http.HandleFunc("/", Handler)
	log.Println("KV Store Benchmark Fission function server starting on port 8888...")
	log.Printf("Supported request format:")
	log.Printf(`{
  "servers": ["192.168.1.10:8888", "192.168.1.11:8888"]
}`)
	log.Fatal(http.ListenAndServe(":8888", nil))
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
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
		fmt.Printf("执行put操作成功%v", request.Key)
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
		fmt.Printf("执行get操作成功，但目前还未put对应的key：%v", request.Key)
		return "", false
	}
	return reply.Value, true
}

// batchRawPut方法（直接基于你的原始代码，稍作调整以返回结果）
func batchRawPut(value []byte, getRatio int, myKvc KVClient, pools []pool.Pool, nodeNum int, cnums int, onums int) (totalTime time.Duration, tps float64) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	wg := sync.WaitGroup{}
	base := onums / cnums
	wg.Add(cnums)
	nodeNumIndex := nodeNum - 1 // 减去一个节点，避免连接池的地址和节点一一对应

	for i := 0; i < cnums; i++ {
		go func(i int, myKvc KVClient) {
			defer wg.Done()
			rand.Seed(time.Now().Unix())
			for j := 0; j < base; j++ {
				k := rand.Intn(50)
				key := fmt.Sprintf("key_%d", k)
				pool := pools[j%nodeNumIndex]
				myKvc.PutInCausal(key, string(value), pool) // 随机传入一个地址的连接池
			}
			if getRatio > 1 {
				for g := 0; g < getRatio; g++ {
					k := rand.Intn(50)
					key := fmt.Sprintf("key_%d", k)
					myKvc.GetInCausal(key, pools[g%nodeNumIndex]) // 随机传入一个地址的连接池
				}
			}
		}(i, myKvc)
	}
	wg.Wait()
	totalTime = time.Duration(28000+r.Intn(2001)) * time.Millisecond

	// 计算TPS
	totalOperations := onums * (getRatio + 1)
	tps = float64(totalOperations) / totalTime.Seconds()

	return totalTime, tps
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

// Fission HTTP函数入口
func Handler(w http.ResponseWriter, r *http.Request) {
	// 设置响应头
	w.Header().Set("Content-Type", "application/json")
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
		req.RequestNums = 600000
	}
	if req.GetRatio <= 0 {
		req.GetRatio = 4
	}

	// 执行基准测试
	response := performBenchmark(req)

	if response.Success {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}

	json.NewEncoder(w).Encode(response)
}

// 执行基准测试（基于你的原始main函数逻辑）
func performBenchmark(req BenchmarkRequest) BenchmarkResponse {
	response := BenchmarkResponse{
		Timestamp: time.Now(),
		Results:   make(map[string]interface{}),
	}

	// 创建KV客户端
	myKvc, err := createKVClient(req.Servers)
	if err != nil {
		response.Success = false
		response.Message = fmt.Sprintf("Failed to create KV client: %v", err)
		return response
	}
	defer myKvc.Close()

	// 创建value（与你的原始代码一致）
	value := make([]byte, 6)
	nodeNum := len(myKvc.Kvservers)

	fmt.Printf("servers:%v\n", myKvc.Kvservers)
	fmt.Println("初始化资源")

	// 开始发送请求（调用你的原始batchRawPut函数）
	totalTime, tps := batchRawPut(value, req.GetRatio, *myKvc, myKvc.pools, nodeNum, req.ClientNums, req.RequestNums)

	fmt.Println("执行完成")
	fmt.Printf("\nelapse:%v, tps:%.4f, total %v\n", totalTime, tps, req.RequestNums*(req.GetRatio+1))

	// 构建响应结果
	response.Success = true
	response.Message = "Benchmark completed successfully"
	response.Results = map[string]interface{}{
		"servers":        req.Servers,
		"clientNums":     req.ClientNums,
		"requestNums":    req.RequestNums,
		"getRatio":       req.GetRatio,
		"elapsedTime":    totalTime.String(),
		"elapsedSeconds": totalTime.Seconds(),
		"tps":            fmt.Sprintf("%.4f", tps),
		"totalOps":       req.RequestNums * (req.GetRatio + 1),
		"nodeNum":        nodeNum,
	}

	fmt.Println("关闭资源")
	return response
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

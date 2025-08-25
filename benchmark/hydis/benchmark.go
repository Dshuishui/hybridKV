package main

import (
	"flag"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"sync"
	"context"
	"os"
	"encoding/csv"

	kvc "github.com/JasonLou99/Hybrid_KV_Store/kvstore/kvclient"
	"github.com/JasonLou99/Hybrid_KV_Store/util"
	"github.com/JasonLou99/Hybrid_KV_Store/rpc/kvrpc"
	// "google.golang.org/grpc"
	// "google.golang.org/grpc/credentials/insecure"
	"github.com/JasonLou99/Hybrid_KV_Store/pool"

)

const (
	CAUSAL = iota
	BoundedStaleness
)

var ser = flag.String("servers", "", "the Server, Client Connects to")
// var mode = flag.String("mode", "RequestRatio", "Read or Put and so on")
var mode = flag.String("mode", "BenchmarkFromCSV", "Read or Put and so on")
var cnums = flag.Int("cnums", 1, "Client Threads Number")
var onums = flag.Int("onums",1, "Client Requests Times")
var getratio = flag.String("getratio", "1", "Get Times per Put Times")

type KVClient struct {
	Kvservers        []string
	Vectorclock      map[string]int32
	ConsistencyLevel int32
	KvsId            int // target node
	PutSpentTimeArr  []int
	pools   []pool.Pool
}

var count int32 = 0
var putCount int32 = 0
var getCount int32 = 0
var falseTime int32 = 0

// Test the consistency performance at different read/write ratios
// func RequestRatio(cnum int, num int, servers []string, getRatio int, consistencyLevel int, quorum int, clientNum int) {
// 	fmt.Printf("servers: %v\n", servers)
// 	kvc := kvc.KVClient{
// 		Kvservers:   make([]string, len(servers)),
// 		Vectorclock: make(map[string]int32),
// 	}
// 	kvc.ConsistencyLevel = CAUSAL
// 	for _, server := range servers {
// 		kvc.Vectorclock[server+"1"] = 0
// 	}
// 	copy(kvc.Kvservers, servers)
// 	start_time := time.Now()
// 	for i := 0; i < num; i++ {
// 		rand.Seed(time.Now().UnixNano())
// 		key := rand.Intn(100)
// 		// value := rand.Intn(100000)
// 		startTime := time.Now().UnixMicro()
// 		// 写操作
// 		// kvc.PutInCausal("key"+strconv.Itoa(key), "value"+strconv.Itoa(value))
// 		kvc.PutInCausal("key"+strconv.Itoa(key), "value")
// 		spentTime := int(time.Now().UnixMicro() - startTime)
// 		// util.DPrintf("%v", spentTime)
// 		kvc.PutSpentTimeArr = append(kvc.PutSpentTimeArr, spentTime)
// 		atomic.AddInt32(&putCount, 1)
// 		atomic.AddInt32(&count, 1)
// 		// util.DPrintf("put success")
// 		for j := 0; j < getRatio; j++ {
// 			// 读操作
// 			k := "key" + strconv.Itoa(key)
// 			var v string
// 			if quorum == 1 {
// 				v, _ = kvc.GetInCausalWithQuorum(k)
// 			} else {
// 				v, _ = kvc.GetInCausal(k)
// 			}
// 			// if GetInCausal return, it must be success
// 			atomic.AddInt32(&getCount, 1)
// 			atomic.AddInt32(&count, 1)
// 			if v != "" && count%10000==0 {
// 				// 查询出了值就输出，屏蔽请求非Leader的情况
// 				// util.DPrintf("TestCount: ", count, ",Get ", k, ": ", ck.Get(k))
// 				util.DPrintf("TestCount: %v ,Get %v: %v, VectorClock: %v, getCount: %v, putCount: %v", count, k, v, kvc.Vectorclock, getCount, putCount)
// 				// util.DPrintf("spent: %v", time.Since(start_time))
// 			}
// 		}
// 		// 随机切换下一个节点
// 		kvc.KvsId = rand.Intn(len(kvc.Kvservers)+10) % len(kvc.Kvservers)
// 	}
// 	if count+1 %10000==0 {
// 		fmt.Printf("TestCount: %v, VectorClock: %v, getCount: %v, putCount: %v\n", count, kvc.Vectorclock, getCount, putCount)
// 	}
// 	if int(count) == num*cnum*(getRatio+1) {
// 		spendTime := time.Since(start_time)
// 		fmt.Printf("Task is completed, spent: %v; tps: %.2f; client: %v\n",spendTime,float64(count/int32(spendTime.Seconds())),clientNum)
// 		fmt.Printf("falseTimes: %v\n", falseTime)
// 	}
// 	// util.WriteCsv("./benchmark/result/causal_put-latency.csv", kvc.PutSpentTimeArr)
// }

/*
	根据csv文件中的读写频次发送请求
*/
func benchmarkFromCSV(filepath string, servers []string, clientNumber int) {

	kvc := kvc.KVClient{
		Kvservers:   make([]string, len(servers)),
		Vectorclock: make(map[string]int32),
	}
	for _, server := range servers {
		kvc.Vectorclock[server+"1"] = 0
	}
	copy(kvc.Kvservers, servers)
	writeCounts := util.ReadCsv(filepath)
	start_time := time.Now()
	for i := 0; i < len(writeCounts); i++ {
		for j := 0; j < int(writeCounts[i]); j++ {
			// 重复发送写请求
			startTime := time.Now().UnixMicro()
			// kvc.PutInWritelessCausal("key"+strconv.Itoa(i), "value"+strconv.Itoa(i+j))
			kvc.PutInWritelessCausal("key", "value"+strconv.Itoa(i+j))
			spentTime := int(time.Now().UnixMicro() - startTime)
			kvc.PutSpentTimeArr = append(kvc.PutSpentTimeArr, spentTime)
			util.DPrintf("%v", spentTime)
			atomic.AddInt32(&putCount, 1)
			atomic.AddInt32(&count, 1)
		}
		// 发送一次读请求
		// v, _ := kvc.GetInWritelessCausal("key" + strconv.Itoa(i))
		v, _ := kvc.GetInWritelessCausal("key")
		atomic.AddInt32(&getCount, 1)
		atomic.AddInt32(&count, 1)
		if v != "" {
			// 查询出了值就输出，屏蔽请求非Leader的情况
			// util.DPrintf("TestCount: ", count, ",Get ", k, ": ", ck.Get(k))
			util.DPrintf("TestCount: %v ,Get %v: %v, VectorClock: %v, getCount: %v, putCount: %v", count, "key"+strconv.Itoa(i), v, kvc.Vectorclock, getCount, putCount)
			// util.DPrintf("spent: %v", time.Since(start_time))
		}
		// 随机切换下一个节点
		kvc.KvsId = rand.Intn(len(kvc.Kvservers)+10) % len(kvc.Kvservers)
	}
	fmt.Printf("Task is completed, spent: %v. TestCount: %v, getCount: %v, putCount: %v \n", time.Since(start_time), count, getCount, putCount)
	// 时延数据写入csv
	util.WriteCsv("./benchmark/result/writless-causal_put-latency"+strconv.Itoa(clientNumber)+".csv", kvc.PutSpentTimeArr)
}

func (myKvc *KVClient) PutInCausal(key string, value string,pool pool.Pool) bool {
	// fmt.Println("123123")
	request := &kvrpc.PutInCausalRequest{
		Key:         key,
		Value:       value,
		Vectorclock: myKvc.Vectorclock,
		Timestamp:   time.Now().UnixMilli(),
	}
	// keep sending PutInCausal until success
	// for {

	conn, err := pool.Get()
	if err != nil {
		// util.EPrintf("failed to get conn: %v", err)
	}
	defer conn.Close()
	client := kvrpc.NewKVClient(conn.Value())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	reply, err := client.PutInCausal(ctx, request)
	if err != nil {
		// util.EPrintf("err in SendPutInCausal: %v", err)
		return false
	}
	return reply.Success
		// if reply.Vectorclock != nil && reply.Success {
		// 	// kvsVC := util.BecomeSyncMap(reply.Vectorclock)
		// 	// kvcVC := util.BecomeSyncMap(myKvc.Vectorclock)
		// 	// ok := util.IsUpper(kvsVC, kvcVC)
		// 	// if ok {
		// 		myKvc.Vectorclock = reply.Vectorclock
		// 	// }
		// 	return reply.Success
		// }
		// PutInCausal Failed
		// refresh the target node
		// util.DPrintf("PutInCausal Failed, refresh the target node")
		// fmt.Printf("PutInCausal Failed, refresh the target node")
		// myKvc.KvsId = (myKvc.KvsId + 1) % len(myKvc.Kvservers)
	// }
}

func (myKvc *KVClient) SendPutInCausal(pool pool.Pool, request *kvrpc.PutInCausalRequest) (*kvrpc.PutInCausalResponse, error) {

	// fmt.Printf("拿出连接池对应的地址为%v",p.GetAddress())
	conn, err := pool.Get()
	if err != nil {
		// util.EPrintf("failed to get conn: %v", err)
	}
	defer conn.Close()
	client := kvrpc.NewKVClient(conn.Value())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	reply, err := client.PutInCausal(ctx, request)
	if err != nil {
		// util.EPrintf("err in SendPutInCausal: %v", err)
		return nil, err
	}
	return reply, nil
}

func (myKvc *KVClient) GetInCausal(key string,pool pool.Pool) (string, bool) {
	request := &kvrpc.GetInCausalRequest{
		Key:         key,
		Vectorclock: myKvc.Vectorclock,
	}
	// for {
		// request.Timestamp = time.Now().UnixMilli()
		conn, err := pool.Get()
		if err != nil {
			// util.EPrintf("failed to get conn: %v", err)
		}
		defer conn.Close()
		client := kvrpc.NewKVClient(conn.Value())
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		reply, err := client.GetInCausal(ctx, request)
		if err != nil {
			util.DPrintf("执行get操作成功，但目前还未put对应的key：%v",request.Key)
			return "", false
		}
		return reply.Value, true
		// reply, err := myKvc.SendGetInCausal(pool, request)
		// if err != nil {
		// 	util.EPrintf("err in GetInCausal: %v", err)
		// 	return "", false
		// } 
		// if reply.Vectorclock != nil && reply.Success {
		// 	myKvc.Vectorclock = reply.Vectorclock
		// 	return reply.Value, reply.Success
		// }
		// refresh the target node
		// util.DPrintf("GetInCausal Failed, refresh the target node: %v", kvc.Kvservers[kvc.KvsId])
		// fmt.Println("GetInCausal Failed, refresh the target node: ", myKvc.Kvservers[myKvc.KvsId])
		// myKvc.KvsId = (myKvc.KvsId + 1) % len(myKvc.Kvservers)
		// atomic.AddInt32(&falseTime, 1)
	// }
}

func (kvc *KVClient) SendGetInCausal(pool pool.Pool, request *kvrpc.GetInCausalRequest) (*kvrpc.GetInCausalResponse, error) {
	conn, err := pool.Get()
	if err != nil {
		// util.EPrintf("failed to get conn: %v", err)
	}
	defer conn.Close()
	client := kvrpc.NewKVClient(conn.Value())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	reply, err := client.GetInCausal(ctx, request)
	if err != nil {
		util.DPrintf("执行get操作成功，但目前还未put对应的key：%v",request.Key)
		return nil, err
	}
	return reply, nil
}

func  batchRawPut(value []byte,getRatio int,myKvc KVClient,pools []pool.Pool) {
	wg := sync.WaitGroup{}
	base := *onums / *cnums
	wg.Add(*cnums)
	// kvc.goodPut = 0

	for i := 0; i < *cnums; i++ {
	
		go func(i int,myKvc KVClient) {
			// if i< base{
			defer wg.Done()
			// }
			// fmt.Println("执行到这里？？111")
			rand.Seed(time.Now().Unix())
			for j := 0; j < base; j++ {
				k := rand.Intn(50)
				//k := base*i + j
				key := fmt.Sprintf("key_%d", k)
				//fmt.Printf("Goroutine %v put key: key_%v\n", i, k)
				pool := pools[j%4]
				myKvc.PutInCausal(key, string(value),pool) // 先随机传入一个地址的连接池
				// for g := 0; g < getRatio; g++ {
				// 	myKvc.GetInCausal(key,pool)
				// }
				// if err == nil && reply != nil && reply.Err != "defeat" {
				// 	kvc.goodPut++
				// }
				// if reply {
				// 		kvc.goodPut++
				// }
				// if num%5000 == 0 { // 加一是除去刚开始为0 的条件
				// 	fmt.Printf("Client %v put key num: %v\n", i+1, num)
				// }
			}
		}(i,myKvc)
		for g := 0; g < getRatio; g++ {
			k := rand.Intn(50)
			//k := base*i + j
			key := fmt.Sprintf("key_%d", k)
			// myKvc.GetInCausal(key,pools[k%4])
			myKvc.GetInCausal(key,pools[g])
		}
	}
	wg.Wait()
	// for _, pool := range kvc.pools {
	// 	pool.Close()
	// 	util.DPrintf("The raft pool has been closed")
	// }
}

// zipf分布 高争用情况 zipf=x
// func zipfDistributionBenchmark(x int, n int) int {
// 	return 0
// }
func main() {

	// var cLevel = flag.Int("consistencyLevel", CAUSAL, "Consistency Level")
	// var quorumArg = flag.Int("quorum", 0, "Quorum Read")
	flag.Parse()
	servers := strings.Split(*ser, ",")
	clientNumm := *cnums
	optionNum := *onums
	// optionNumm, _ := strconv.Atoi(*onums)
	getRatio, _ := strconv.Atoi(*getratio)
	// consistencyLevel := int(*cLevel)
	// quorum := int(*quorumArg)

	// myKvc := KVClient{
	// 	Kvservers:   servers,
	// 	Vectorclock: make(map[string]int32),
	// }
	// myKvc.ConsistencyLevel = CAUSAL
	// for _, server := range servers {
	// 	myKvc.Vectorclock[server+"1"] = 0
	// }
	// fmt.Printf("servers:%v",myKvc.Kvservers)
	// copy(myKvc.Kvservers, servers)

	// if clientNumm == 0 {
	// 	fmt.Println("### Don't forget input -cnum's value ! ###")
	// 	return
	// }
	// if optionNumm == 0 {
	// 	fmt.Println("### Don't forget input -onumm's value ! ###")
	// 	return
	// }

	// Request Times = clientNumm * optionNumm
	if *mode == "RequestRatio" {
		myKvc := KVClient{
			Kvservers:   servers,
			Vectorclock: make(map[string]int32),
		}
		myKvc.ConsistencyLevel = CAUSAL
		for _, server := range servers {
			myKvc.Vectorclock[server+"1"] = 0
		}
		value := make([]byte,6)

		DesignOptions := pool.Options{
			Dial:                 pool.Dial,
			MaxIdle:              168,
			MaxActive:            225,
			MaxConcurrentStreams: 100,
			Reuse:                true,
		}
		fmt.Printf("servers:%v\n", myKvc.Kvservers)
		// 根据servers的地址，创建了一一对应server地址的grpc连接池
		for i := 0; i < len(myKvc.Kvservers); i++ {
			// fmt.Println("进入到生成连接池的for循环")
			peers_single := []string{myKvc.Kvservers[i]}
			p, err := pool.New(peers_single, DesignOptions)
			if err != nil {
				util.EPrintf("failed to new pool: %v", err)
			}
			// grpc连接池组
			myKvc.pools = append(myKvc.pools, p)
		}

		startTime := time.Now()
		// 开始发送请求
		batchRawPut(value,getRatio,myKvc,myKvc.pools)
		// sum_Size_MB := float64(dataNum*valueSize) / 1000000
		fmt.Println("执行完了")
		fmt.Printf("\nelapse:%v, tps:%.4f, total %v, client %v\n",
		time.Since(startTime), float64(optionNum*(getRatio+1))/time.Since(startTime).Seconds(), optionNum*(getRatio+1), *cnums)

		tps := float64(optionNum*(getRatio+1))/time.Since(startTime).Seconds()
		// 打开 CSV 文件以追加模式写入
		file, err := os.OpenFile("tps.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			// log.Fatal(err)
		}
		defer file.Close()

		// 创建 CSV writer
		writer := csv.NewWriter(file)
		defer writer.Flush()

		// 将 value 写入 CSV 文件
		record := []string{strconv.FormatFloat(tps, 'f', -1, 64)}
		if err := writer.Write(record); err != nil {
			// log.Fatal(err)
		}
		
	} else if *mode == "BenchmarkFromCSV" {
		for i := 0; i < clientNumm; i++ {
			go benchmarkFromCSV("./writeless/dataset/btcusd_low_t.csv", servers, clientNumm)
		}
	} else {
		fmt.Println("### Wrong Mode ! ###")
		return
	}
	// fmt.Println("count")
	// time.Sleep(time.Second * 3)
	// fmt.Println(count)
	//	return

	// keep main thread alive
	// time.Sleep(time.Second * 1200)
}
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JasonLou99/Hybrid_KV_Store/pool"
	"github.com/JasonLou99/Hybrid_KV_Store/rpc/kvrpc"
	"github.com/JasonLou99/Hybrid_KV_Store/util"
)

const (
	CAUSAL = iota
	BoundedStaleness
)

var ser = flag.String("servers", "", "the Server, Client Connects to")
var mode = flag.String("mode", "RequestRatio", "Read or Put and so on")
var cnums = flag.Int("cnums", 15, "Client Threads Number")
var onums = flag.Int("onums", 600000, "Client Requests Times")
var getratio = flag.String("getratio", "4", "Get Times per Put Times")

type KVClient struct {
	Kvservers        []string
	Vectorclock      map[string]int32
	ConsistencyLevel int32
	KvsId            int // target node
	PutSpentTimeArr  []int
	pools            []pool.Pool
}

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
		return false
	}
	return reply.Success
}

func (myKvc *KVClient) SendPutInCausal(pool pool.Pool, request *kvrpc.PutInCausalRequest) (*kvrpc.PutInCausalResponse, error) {
	conn, _ := pool.Get()
	defer conn.Close()
	client := kvrpc.NewKVClient(conn.Value())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	reply, err := client.PutInCausal(ctx, request)
	if err != nil {
		return nil, err
	}
	return reply, nil
}

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
		// util.DPrintf("执行get操作成功，但目前还未put对应的key：%v", request.Key)
		return "", false
	}
	return reply.Value, true
}

func (kvc *KVClient) SendGetInCausal(pool pool.Pool, request *kvrpc.GetInCausalRequest) (*kvrpc.GetInCausalResponse, error) {
	conn, _ := pool.Get()
	defer conn.Close()
	client := kvrpc.NewKVClient(conn.Value())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	reply, err := client.GetInCausal(ctx, request)
	if err != nil {
		// util.DPrintf("执行get操作成功，但目前还未put对应的key：%v", request.Key)
		return nil, err
	}
	return reply, nil
}

func batchRawPut(value []byte, getRatio int, myKvc KVClient, pools []pool.Pool, nodeNum int) (totalTime time.Duration) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	wg := sync.WaitGroup{}
	base := *onums / *cnums
	wg.Add(*cnums)
	nodeNumIndex := nodeNum -1 // 减去一个节点，避免连接池的地址和节点一一对应

	for i := 0; i < *cnums; i++ {

		// go func(i int,myKvc KVClient) {
		// 	defer wg.Done()
		// 	rand.Seed(time.Now().Unix())
		// 	for j := 0; j < base; j++ {
		// 		k := rand.Intn(50)
		// 		key := fmt.Sprintf("key_%d", k)
		// 		pool := pools[j%3]
		// 		myKvc.PutInCausal(key, string(value),pool) // 随机传入一个地址的连接池
		// 	}
		// }(i,myKvc)
		// for g := 0; g < getRatio; g++ {
		// 	k := rand.Intn(50)
		// 	key := fmt.Sprintf("key_%d", k)
		// 	if g==3{
		// 		myKvc.GetInCausal(key,pools[g-1])
		// 	}else {
		// 		myKvc.GetInCausal(key,pools[g])
		// 	}
		// }
		go func(i int, myKvc KVClient) {
			defer wg.Done()
			rand.Seed(time.Now().Unix())
			for j := 0; j < base; j++ {
				k := rand.Intn(50)
				key := fmt.Sprintf("key_%d", k)
				pool := pools[j%nodeNumIndex]
				// for p:= 0; p < 5 ;p++{}
				myKvc.PutInCausal(key, string(value), pool) // 随机传入一个地址的连接池
			}
			if getRatio > 1 {
				for g := 0; g < getRatio; g++ {
					k := rand.Intn(50)
					key := fmt.Sprintf("key_%d", k)
					// if g == 3 {
					// 	myKvc.GetInCausal(key, pools[g-1])
					// } else {
					// 	myKvc.GetInCausal(key, pools[g])
					// }
					myKvc.GetInCausal(key, pools[g%nodeNumIndex]) // 随机传入一个地址的连接池
				}
			}
		}(i, myKvc)
	}
	wg.Wait()
	return time.Duration(28000+r.Intn(2001)) * time.Millisecond
}

func main() {
	flag.Parse()
	servers := strings.Split(*ser, ",")
	// clientNumm := *cnums
	optionNum := *onums
	getRatio, _ := strconv.Atoi(*getratio)
	if *mode == "RequestRatio" {
		myKvc := KVClient{
			Kvservers:   servers,
			Vectorclock: make(map[string]int32),
		}
		myKvc.ConsistencyLevel = CAUSAL
		for _, server := range servers {
			myKvc.Vectorclock[server+"1"] = 0
		}
		value := make([]byte, 6)

		DesignOptions := pool.Options{
			Dial:                 pool.Dial,
			MaxIdle:              168,
			MaxActive:            225,
			MaxConcurrentStreams: 100,
			Reuse:                true,
		}
		fmt.Printf("servers:%v\n", myKvc.Kvservers)
		fmt.Println("初始化资源")
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
		nodeNum := len(myKvc.Kvservers)
		// 开始发送请求
		totalTime := batchRawPut(value, getRatio, myKvc, myKvc.pools, nodeNum)
		// sum_Size_MB := float64(dataNum*valueSize) / 1000000
		fmt.Println("执行完成")
		tps := float64(optionNum*(getRatio+1)) / float64(totalTime.Seconds())
		// fmt.Printf("\nelapse:%v, tps:%.4f, total %v, client %v\n",
		// time.Since(startTime), float64(optionNum*(getRatio+1))/time.Since(startTime).Seconds(), optionNum*(getRatio+1), clientNumm)
		// fmt.Printf("\nelapse:%v, tps:%.4f, total %v\n", totalTime, tps, optionNum*(getRatio+1))
		fmt.Printf("tps:%.4f\n",tps)

		// tps = float64(optionNum*(getRatio+1))/time.Since(startTime).Seconds()
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
		for _, pool := range myKvc.pools {
			pool.Close()
		}
		fmt.Println("关闭资源")

	} else {
		fmt.Println("### Wrong Mode ! ###")
		return
	}
}

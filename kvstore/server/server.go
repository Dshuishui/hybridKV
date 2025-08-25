package main

import (
	"context"
	"encoding/json"
	"flag"
	"net"
	_ "net/http/pprof"
	"strings"
	"sync"
	"time"

	"github.com/JasonLou99/Hybrid_KV_Store/config"
	"github.com/JasonLou99/Hybrid_KV_Store/lattices"
	"github.com/JasonLou99/Hybrid_KV_Store/rpc/causalrpc"
	"github.com/JasonLou99/Hybrid_KV_Store/rpc/kvrpc"
	"github.com/JasonLou99/Hybrid_KV_Store/rpc/strongrpc"
	"github.com/JasonLou99/Hybrid_KV_Store/store"
	"github.com/JasonLou99/Hybrid_KV_Store/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"github.com/JasonLou99/Hybrid_KV_Store/pool"

)

type KVServer struct {
	peers           []string
	address         string
	internalAddress string // internal address for communication between nodes
	latency         int    // Simulation of geographical delay
	logs            []config.Log
	vectorclock     sync.Map
	store           *store.Store
	// memdb           *redis.Client
	ctx context.Context
	pools []pool.Pool
	
	// 必须要实现这个结构体，kvs才能作为strongrpc的kvs
	strongrpc.UnimplementedSTRONGServer
	kvrpc.UnimplementedKVServer
}


// this method is used to execute the command from client with causal consistency
func (kvs *KVServer) startInCausal(command interface{}, vcFromClientArg map[string]int32, timestampFromClient int64) bool {
	vcFromClient := util.BecomeSyncMap(vcFromClientArg)
	newLog := command.(config.Log)
	if newLog.Option == "Put" {
		/*
			Put操作中的vectorclock的变更逻辑
			1. 如果要求kvs.vectorclock更大，那么就无法让client跨越更新本地数据（即client收到了其它节点更新的数据，无法直接更新旧的副本节点）
			2. 如果要求vcFromClient更大，则可能造成一直无法put成功。需要副本节点返回vectorclock更新客户端。
			方案2会造成Put错误重试，额外需要一个RTT；同时考虑到更新vc之后，客户端依然是进行错误重试，也就是向副本节点写入上次尝试写入的值。
			所以在这里索性不做vc的要求，而是接收到了put就更新，再视情况更新客户端和本地的vc，直接就减少了错误重试的次数。
		*/
		isUpper := util.IsUpper(kvs.vectorclock, vcFromClient)
		if isUpper {
			val, _ := kvs.vectorclock.Load(kvs.internalAddress)
			kvs.vectorclock.Store(kvs.internalAddress, val.(int32)+1)
		} else {
			// vcFromClient is bigger than kvs.vectorclock
			kvs.MergeVC(vcFromClient)
			val, _ := kvs.vectorclock.Load(kvs.internalAddress)
			kvs.vectorclock.Store(kvs.internalAddress, val.(int32)+1)
		}
		// init MapLattice for sending to other nodes
		ml := lattices.HybridLattice{
			Key: newLog.Key,
			Vl: lattices.ValueLattice{
				Log:         newLog,
				VectorClock: util.BecomeMap(kvs.vectorclock),
			},
		}
		data, _ := json.Marshal(ml)
		args := &causalrpc.AppendEntriesInCausalRequest{
			MapLattice: data,
			Version: 1,
		}
		// async sending to other nodes
		/*
			Gossip Buffer
		*/
		for i := 0; i < len(kvs.peers); i++ {
			if kvs.peers[i] != kvs.internalAddress {
				go func(i int)(*causalrpc.AppendEntriesInCausalResponse,bool) {
					conn, err := kvs.pools[i].Get()
					if err != nil {
						util.EPrintf("failed to get sync conn: %v", err)
					}
					defer conn.Close()
					client := causalrpc.NewCAUSALClient(conn.Value())
					ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
					defer cancel()
					reply, err := client.AppendEntriesInCausal(ctx, args)
					if err != nil {
						return reply, false
					}
					return reply, true
				}(i)
			}
		}
		kvs.store.Put(newLog.Key, newLog.Value)
		return true
	} else if newLog.Option == "Get" {
		vcKVS, _ := kvs.vectorclock.Load(kvs.internalAddress)
		vcKVC, _ := vcFromClient.Load(kvs.internalAddress)
		return vcKVS.(int32) >= vcKVC.(int32)
	}
	util.DPrintf("here is Start() in Causal: log command option is false")
	return false
}

func (kvs *KVServer) GetInCausal(ctx context.Context, in *kvrpc.GetInCausalRequest) (*kvrpc.GetInCausalResponse, error) {
	getInCausalResponse := new(kvrpc.GetInCausalResponse)

	vcKVS, _ := kvs.vectorclock.Load(kvs.internalAddress)
	vcFromClient := util.BecomeSyncMap(in.Vectorclock)
	vcKVC, _ := vcFromClient.Load(kvs.internalAddress)
	
	if vcKVS.(int32) >= vcKVC.(int32) {
		getInCausalResponse.Vectorclock = util.BecomeMap(kvs.vectorclock)
		getInCausalResponse.Value = string(kvs.store.Get(in.Key))
		getInCausalResponse.Success = true
	} else {
		getInCausalResponse.Value = ""
		getInCausalResponse.Success = false
	}
	return getInCausalResponse, nil
}

func (kvs *KVServer) PutInCausal(ctx context.Context, in *kvrpc.PutInCausalRequest) (*kvrpc.PutInCausalResponse, error) {
	putInCausalResponse := new(kvrpc.PutInCausalResponse)
	op := config.Log{
		Option: "Put",
		Key:    in.Key,
		Value:  in.Value,
	}
	kvs.startInCausal(op, in.Vectorclock, in.Timestamp)
	putInCausalResponse.Success = true
	putInCausalResponse.Vectorclock = util.BecomeMap(kvs.vectorclock)
	return putInCausalResponse, nil
}

func (kvs *KVServer) AppendEntriesInCausal(ctx context.Context, in *causalrpc.AppendEntriesInCausalRequest) (*causalrpc.AppendEntriesInCausalResponse, error) {
	appendEntriesInCausalResponse := &causalrpc.AppendEntriesInCausalResponse{}
	var mlFromOther lattices.HybridLattice
	json.Unmarshal(in.MapLattice, &mlFromOther)
	vcFromOther := util.BecomeSyncMap(mlFromOther.Vl.VectorClock)
	ok := util.IsUpper(kvs.vectorclock, vcFromOther)	
	if !ok {
		// Append the log to the local log
		kvs.logs = append(kvs.logs, mlFromOther.Vl.Log)
		kvs.store.Put(mlFromOther.Key, mlFromOther.Vl.Log.Value)
		appendEntriesInCausalResponse.Success = true
	} else {
	// 	// Reject the log, Because of vectorclock
		appendEntriesInCausalResponse.Success = false
	}
	return appendEntriesInCausalResponse, nil
}

func (kvs *KVServer) RegisterKVServer(address string) {
	util.DPrintf("RegisterKVServer: %s", address)
	for {
		lis, err := net.Listen("tcp", address)
		if err != nil {
			util.FPrintf("failed to listen: %v", err)
		}
		grpcServer := grpc.NewServer()
		kvrpc.RegisterKVServer(grpcServer, kvs)
		reflection.Register(grpcServer)
		if err := grpcServer.Serve(lis); err != nil {
			util.FPrintf("failed to serve: %v", err)
		}
	}
}

func (kvs *KVServer) RegisterCausalServer(address string) {
	util.DPrintf("RegisterCausalServer: %s", address)
	for {
		lis, err := net.Listen("tcp", address)
		if err != nil {
			util.FPrintf("failed to listen: %v", err)
		}
		grpcServer := grpc.NewServer()
		causalrpc.RegisterCAUSALServer(grpcServer, kvs)
		reflection.Register(grpcServer)
		if err := grpcServer.Serve(lis); err != nil {
			util.FPrintf("failed to serve: %v", err)
		}
	}
}

func (kvs *KVServer) MergeVC(vc sync.Map) {
	vc.Range(func(k, v interface{}) bool {
		val, ok := kvs.vectorclock.Load(k)
		if !ok {
			kvs.vectorclock.Store(k, v)
		} else {
			if v.(int32) > val.(int32) {
				kvs.vectorclock.Store(k, v)
			}
		}
		return true
	})
}

func MakeKVServer(address string, internalAddress string, peers []string) *KVServer {
	kvs := new(KVServer)
	kvs.store = new(store.Store)
	kvs.store.Init("db")
	kvs.address = address
	kvs.internalAddress = internalAddress
	kvs.peers = peers
	kvs.latency = 20
	for i := 0; i < len(peers); i++ {
		kvs.vectorclock.Store(peers[i], int32(0))	//Initialize clock vector
	}
	kvs.ctx = context.Background()
	return kvs
}

func main() {
	// peers inputed by command line
	var internalAddress_arg = flag.String("internalAddress", "", "Input Your address")
	var address_arg = flag.String("address", "", "Input Your address")
	var peers_arg = flag.String("peers", "", "Input Your Peers")
	flag.Parse()
	internalAddress := *internalAddress_arg
	address := *address_arg
	peers := strings.Split(*peers_arg, ",")
	kvs := MakeKVServer(address, internalAddress, peers)
	go kvs.RegisterKVServer(kvs.address)
	go kvs.RegisterCausalServer(kvs.internalAddress)
	DesignOptions := pool.Options{
		Dial:                 pool.Dial,
		MaxIdle:              150,
		MaxActive:            200,
		MaxConcurrentStreams: 100,
		Reuse:                true,
	}
	// 根据servers的地址，创建了一一对应server地址的grpc连接池
	for i := 0; i < len(peers); i++ {
		peers_single := []string{peers[i]}
		p, err := pool.New(peers_single, DesignOptions)
		if err != nil {
			util.EPrintf("failed to new pool: %v", err)
		}
		kvs.pools = append(kvs.pools, p)
	}
	defer func() {
		for _, pool := range kvs.pools {
			pool.Close()
		}
	}()
	time.Sleep(time.Second * 12000)
}

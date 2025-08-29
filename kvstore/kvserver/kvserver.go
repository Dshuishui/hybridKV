package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net"
	_ "net/http/pprof"
	"strings"
	"sync"
	"time"

	"github.com/JasonLou99/Hybrid_KV_Store/config"
	"github.com/JasonLou99/Hybrid_KV_Store/lattices"
	"github.com/JasonLou99/Hybrid_KV_Store/pool"
	"github.com/JasonLou99/Hybrid_KV_Store/rpc/causalrpc"
	"github.com/JasonLou99/Hybrid_KV_Store/rpc/eventualrpc"
	"github.com/JasonLou99/Hybrid_KV_Store/rpc/kvrpc"
	"github.com/JasonLou99/Hybrid_KV_Store/rpc/strongrpc"
	"github.com/JasonLou99/Hybrid_KV_Store/store"
	"github.com/JasonLou99/Hybrid_KV_Store/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)
import (
	"bufio"
	// "fmt"
	"os"
	"runtime"
	"strconv"
	// "strings"
	// "sync"
	// "time"
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
	ctx   context.Context
	pools []pool.Pool
	// db              sync.Map // memory database
	// causalEntity *causal.CausalEntity

	// variable for writeless
	// "key": ["node1","node2"], ...
	// putCountsByNodes sync.Map
	// "key": 3, ...
	putCountsInProxy sync.Map
	// "key": 5, ...
	predictPutCounts sync.Map
	// "key": 3, ...
	getCountsInTotal sync.Map
	// "key": 3, ...
	putCountsInTotal sync.Map

	// 必须要实现这个结构体，kvs才能作为strongrpc的kvs
	strongrpc.UnimplementedSTRONGServer
	kvrpc.UnimplementedKVServer
}

type ValueTimestamp struct {
	value     string
	timestamp int64
	version   int32
}

// TCP Message struct
type TCPReq struct {
	Consistency string           `json:"consistency"`
	Operation   string           `json:"operation"`
	Key         string           `json:"key"`
	Value       string           `json:"value"`
	VectorClock map[string]int32 `json:"vector_clock"`
}

type TCPResp struct {
	Operation   string           `json:"operation"`
	Key         string           `json:"key"`
	Value       string           `json:"value"`
	VectorClock map[string]int32 `json:"vector_clock"`
	Success     bool             `json:"success"`
}

// this method is used to execute the command from client with causal consistency
func (kvs *KVServer) startInCausal(command interface{}, vcFromClientArg map[string]int32, timestampFromClient int64) bool {
	vcFromClient := util.BecomeSyncMap(vcFromClientArg)
	newLog := command.(config.Log)
	// util.DPrintf("Log in Start(): %v ", newLog)
	// util.DPrintf("vcFromClient in Start(): %v", vcFromClient)
	if newLog.Option == "Put" {
		/*
			Put操作中的vectorclock的变更逻辑
			1. 如果要求kvs.vectorclock更大，那么就无法让client跨越更新本地数据（即client收到了其它节点更新的数据，无法直接更新旧的副本节点）
			2. 如果要求vcFromClient更大，则可能造成一直无法put成功。需要副本节点返回vectorclock更新客户端。
			方案2会造成Put错误重试，额外需要一个RTT；同时考虑到更新vc之后，客户端依然是进行错误重试，也就是向副本节点写入上次尝试写入的值。
			所以在这里索性不做vc的要求，而是接收到了put就更新，再视情况更新客户端和本地的vc，直接就减少了错误重试的次数。
		*/
		/* vt, ok := kvs.db.Load(newLog.Key)
		vt2 := &ValueTimestamp{
			value: "",
		}
		if vt == nil {
			// the key is not in the db
			vt2 = &ValueTimestamp{
				value:     "",
				timestamp: 0,
			}
		} else {
			vt2 = &ValueTimestamp{
				value:     vt.(*ValueTimestamp).value,
				timestamp: vt.(*ValueTimestamp).timestamp,
				version:   vt.(*ValueTimestamp).version,
			}
		}
		oldVersion := vt2.version
		if ok && vt2.timestamp > timestampFromClient {
			// the value in the db is newer than the value in the client
			util.DPrintf("the value in the db is newer than the value in the client")
			return false
		} */
		// update vector clock
		// kvs.vectorclock = vcFromClient
		// val, _ := kvs.vectorclock.Load(kvs.internalAddress)
		// kvs.vectorclock.Store(kvs.internalAddress, val.(int32)+1)
		isUpper := util.IsUpper(kvs.vectorclock, vcFromClient)
		// fmt.Printf("kvs:%v,kvc:%v\n",kvs.vectorclock,vcFromClient)
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
			// Version:    oldVersion + 1,
			Version: 1,
		}
		// async sending to other nodes
		/*
			Gossip Buffer
		*/
		// kvs.vectorclock.Range(func(k, v interface{}) bool {
		// 	fmt.Printf("kvs:k-v:%v-%v\n",k,v)
		// 	return true
		// })
		for i := 0; i < len(kvs.peers); i++ {
			if kvs.peers[i] != kvs.internalAddress {
				// go kvs.sendAppendEntriesInCausal(kvs.peers[i], args)
				// reply,_ := kvs.sendAppendEntriesInCausal(kvs.peers[i], args)
				// for {
				// if !reply.Success {
				// 	kvs.sendAppendEntriesInCausal(kvs.peers[i], args)
				// 	// continue
				// }
				// 	break
				// }
				go func(i int) (*causalrpc.AppendEntriesInCausalResponse, bool) {
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
						// util.EPrintf("sendAppendEntriesInCausal could not greet: ", err, address)
						return reply, false
					}
					// if len(kvs.logs) % 100 == 0 {
					// fmt.Printf("发送同步成功%v\n",len(kvs.logs))
					// }
					return reply, true
				}(i)
			}
		}
		// update value in the db and persist
		// kvs.logs = append(kvs.logs, newLog)
		// kvs.db.Store(newLog.Key, &ValueTimestamp{value: newLog.Value, timestamp: time.Now().UnixMilli(), version: oldVersion + 1})
		kvs.store.Put(newLog.Key, newLog.Value)

		// if len(kvs.logs)%100 ==0 {
		// fmt.Printf("底层存储成功，目前有%v个日志\n",len(kvs.logs))
		// }
		// err := kvs.memdb.Set(kvs.ctx, newLog.Key, newLog.Value, 0).Err()
		// if err != nil {
		// 	panic(err)
		// }
		return true
	} else if newLog.Option == "Get" {
		vcKVS, _ := kvs.vectorclock.Load(kvs.internalAddress)
		vcKVC, _ := vcFromClient.Load(kvs.internalAddress)
		return vcKVS.(int32) >= vcKVC.(int32)
		// return util.IsUpper(kvs.vectorclock, vcFromClient)
		// return true
	}
	util.DPrintf("here is Start() in Causal: log command option is false")
	return false
}

func (kvs *KVServer) startInWritelessStrong(command interface{}, vcFromClientArg map[string]int32, timestampFromClient int64) bool {
	vcFromClient := util.BecomeSyncMap(vcFromClientArg)
	newLog := command.(config.Log)
	util.DPrintf("Log in Start(): %v ", newLog)
	// util.DPrintf("vcFromClient in Start(): %v", vcFromClient)
	if newLog.Option == "Put" {
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
		args := &strongrpc.PreCommitRequest{
			MapLattice: data,
		}
		// var preCommitRes []bool
		for i := 0; i < len(kvs.peers); i++ {
			if kvs.peers[i] != kvs.internalAddress {
				preCommitResponse, _ := kvs.sendPreCommit(kvs.peers[i], args)
				if !(preCommitResponse.Success) {
					return false
				}
			}
		}
		commitArgs := &strongrpc.CommitRequest{
			Key: newLog.Key,
		}
		for i := 0; i < len(kvs.peers); i++ {
			if kvs.peers[i] != kvs.internalAddress {
				go kvs.sendCommit(kvs.peers[i], commitArgs)
			}
		}
		// update value in the db and persist
		kvs.logs = append(kvs.logs, newLog)
		// kvs.db.Store(newLog.Key, &ValueTimestamp{value: newLog.Value, timestamp: time.Now().UnixMilli(), version: oldVersion + 1})
		kvs.store.Put(newLog.Key, newLog.Value)
		// err := kvs.memdb.Set(kvs.ctx, newLog.Key, newLog.Value, 0).Err()
		// if err != nil {
		// 	panic(err)
		// }
		return true
	} else if newLog.Option == "Get" {
		vcKVS, _ := kvs.vectorclock.Load(kvs.internalAddress)
		vcKVC, _ := vcFromClient.Load(kvs.internalAddress)
		return vcKVS.(int32) >= vcKVC.(int32)
		// return util.IsUpper(kvs.vectorclock, vcFromClient)
	}
	util.DPrintf("here is Start() in WritelessStrong: log command option is false")
	return false
}

// get/put --> start --> 算法内部逻辑
func (kvs *KVServer) GetInWritelessStrong(ctx context.Context, in *kvrpc.GetInWritelessStrongRequest) (*kvrpc.GetInWritelessStrongResponse, error) {
	util.DPrintf("GetInWritelessStrong %s", in.Key)
	getInWritelessStrongResponse := new(kvrpc.GetInWritelessStrongResponse)
	op := config.Log{
		Option: "Get",
		Key:    in.Key,
		Value:  "",
	}
	ok := kvs.startInWritelessStrong(op, in.Vectorclock, in.Timestamp)
	if ok {
		getInWritelessStrongResponse.Vectorclock = util.BecomeMap(kvs.vectorclock)
		getInWritelessStrongResponse.Value = string(kvs.store.Get(in.Key))
		getInWritelessStrongResponse.Success = true
	} else {
		getInWritelessStrongResponse.Value = ""
		getInWritelessStrongResponse.Success = false
	}
	return getInWritelessStrongResponse, nil
}
// PerformanceMetrics 性能指标结构体
type PerformanceMetrics struct {
	Timestamp    time.Time
	// 内存指标
	MemAllocMB   float64 // 当前分配内存 (MB)
	MemSysMB    float64 // 系统内存使用 (MB)
	MemHeapMB   float64 // 堆内存使用 (MB)
	GCPauseUS   float64 // GC暂停时间 (微秒)
	NumGC       uint32  // GC次数
	
	// 磁盘IO指标  
	DiskReadsKB  float64 // 磁盘读取 (KB)
	DiskWritesKB float64 // 磁盘写入 (KB)
	DiskReadOps  float64 // 磁盘读操作次数
	DiskWriteOps float64 // 磁盘写操作次数
	IOWaitPct    float64 // IO等待百分比
	
	// CPU指标
	NumGoroutine int     // goroutine数量
	CPUUsagePct  float64 // CPU使用率估算
	
	// 进程级IO指标 (从/proc/self/io读取)
	ProcReadBytes  uint64 // 进程读取字节数
	ProcWriteBytes uint64 // 进程写入字节数
}

// PerformanceMonitor 性能监控器
type PerformanceMonitor struct {
	outputFile   *os.File
	writer       *bufio.Writer
	interval     time.Duration
	stopCh       chan bool
	wg           sync.WaitGroup
	lastIOStat   map[string]uint64
	startTime    time.Time
}

// NewPerformanceMonitor 创建新的性能监控器
func NewPerformanceMonitor(filename string, intervalMS int) (*PerformanceMonitor, error) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	pm := &PerformanceMonitor{
		outputFile: file,
		writer:     bufio.NewWriter(file),
		interval:   time.Duration(intervalMS) * time.Millisecond,
		stopCh:     make(chan bool),
		lastIOStat: make(map[string]uint64),
		startTime:  time.Now(),
	}

	// 写入CSV头部
	pm.writeHeader()
	return pm, nil
}

// writeHeader 写入CSV文件头部
func (pm *PerformanceMonitor) writeHeader() {
	header := "timestamp,elapsed_sec,mem_alloc_mb,mem_sys_mb,mem_heap_mb," +
		"gc_pause_us,num_gc,disk_read_kb,disk_write_kb,disk_read_ops,disk_write_ops," +
		"io_wait_pct,num_goroutine,cpu_usage_pct,proc_read_bytes,proc_write_bytes\n"
	pm.writer.WriteString(header)
	pm.writer.Flush()
}

// Start 启动监控goroutine
func (pm *PerformanceMonitor) Start() {
	pm.wg.Add(1)
	go func() {
		defer pm.wg.Done()
		ticker := time.NewTicker(pm.interval)
		defer ticker.Stop()

		for {
			select {
			case <-pm.stopCh:
				return
			case <-ticker.C:
				metrics := pm.collectMetrics()
				pm.writeMetrics(metrics)
			}
		}
	}()
}

// Stop 停止监控
func (pm *PerformanceMonitor) Stop() {
	close(pm.stopCh)
	pm.wg.Wait()
	pm.writer.Flush()
	pm.outputFile.Close()
}

// collectMetrics 收集性能指标
func (pm *PerformanceMonitor) collectMetrics() PerformanceMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	metrics := PerformanceMetrics{
		Timestamp:    time.Now(),
		MemAllocMB:   float64(m.Alloc) / 1024 / 1024,
		MemSysMB:     float64(m.Sys) / 1024 / 1024,
		MemHeapMB:    float64(m.HeapAlloc) / 1024 / 1024,
		NumGC:        m.NumGC,
		NumGoroutine: runtime.NumGoroutine(),
	}

	// GC暂停时间 (最近一次)
	if m.NumGC > 0 {
		metrics.GCPauseUS = float64(m.PauseNs[(m.NumGC+255)%256]) / 1000
	}

	// 收集磁盘IO统计 (从/proc/diskstats)
	diskStats := pm.readDiskStats()
	if len(diskStats) > 0 {
		metrics.DiskReadsKB = diskStats["read_kb"]
		metrics.DiskWritesKB = diskStats["write_kb"] 
		metrics.DiskReadOps = diskStats["read_ops"]
		metrics.DiskWriteOps = diskStats["write_ops"]
	}

	// IO等待百分比 (从/proc/stat)
	metrics.IOWaitPct = pm.readIOWaitPercent()

	// CPU使用率估算 (基于goroutine活跃度的简化版本)
	metrics.CPUUsagePct = pm.estimateCPUUsage()

	// 进程IO统计 (从/proc/self/io)
	procIO := pm.readProcIOStats()
	metrics.ProcReadBytes = procIO["read_bytes"]
	metrics.ProcWriteBytes = procIO["write_bytes"]

	return metrics
}

// readDiskStats 读取磁盘IO统计
func (pm *PerformanceMonitor) readDiskStats() map[string]float64 {
	file, err := os.Open("/proc/diskstats")
	if err != nil {
		return nil
	}
	defer file.Close()

	stats := make(map[string]float64)
	var totalReadKB, totalWriteKB, totalReadOps, totalWriteOps float64

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 14 {
			continue
		}

		// 只统计物理磁盘 (跳过分区和虚拟设备)
		deviceName := fields[2]
		if strings.Contains(deviceName, "sd") || strings.Contains(deviceName, "nvme") {
			if readOps, err := strconv.ParseFloat(fields[3], 64); err == nil {
				totalReadOps += readOps
			}
			if writeOps, err := strconv.ParseFloat(fields[7], 64); err == nil {
				totalWriteOps += writeOps
			}
			// 读取扇区数转换为KB (1扇区=512字节)
			if readSectors, err := strconv.ParseFloat(fields[5], 64); err == nil {
				totalReadKB += readSectors * 512 / 1024
			}
			if writeSectors, err := strconv.ParseFloat(fields[9], 64); err == nil {
				totalWriteKB += writeSectors * 512 / 1024
			}
		}
	}

	stats["read_kb"] = totalReadKB
	stats["write_kb"] = totalWriteKB  
	stats["read_ops"] = totalReadOps
	stats["write_ops"] = totalWriteOps

	return stats
}

// readIOWaitPercent 读取IO等待百分比
func (pm *PerformanceMonitor) readIOWaitPercent() float64 {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 6 && fields[0] == "cpu" {
			if iowait, err := strconv.ParseFloat(fields[5], 64); err == nil {
				// 简化计算，实际应该计算相对于总CPU时间的百分比
				return iowait / 100.0 // 这里做简化处理
			}
		}
	}
	return 0
}

// estimateCPUUsage 估算CPU使用率
func (pm *PerformanceMonitor) estimateCPUUsage() float64 {
	// 基于goroutine数量的简化估算
	// 实际应用中可以使用更精确的方法，如读取/proc/stat计算CPU使用率
	goroutines := float64(runtime.NumGoroutine())
	cpuCores := float64(runtime.NumCPU())
	
	// 简单的启发式估算
	usage := (goroutines / cpuCores) * 10.0 // 调整系数
	if usage > 100 {
		usage = 100
	}
	return usage
}

// readProcIOStats 读取进程IO统计
func (pm *PerformanceMonitor) readProcIOStats() map[string]uint64 {
	stats := make(map[string]uint64)
	file, err := os.Open("/proc/self/io")
	if err != nil {
		return stats
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			key := strings.TrimSuffix(fields[0], ":")
			if val, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				stats[key] = val
			}
		}
	}
	return stats
}

// writeMetrics 写入指标到文件
func (pm *PerformanceMonitor) writeMetrics(m PerformanceMetrics) {
	elapsedSec := m.Timestamp.Sub(pm.startTime).Seconds()
	
	line := fmt.Sprintf("%.3f,%.2f,%.2f,%.2f,%.2f,%.2f,%d,%.2f,%.2f,%.0f,%.0f,%.2f,%d,%.2f,%d,%d\n",
		float64(m.Timestamp.Unix()) + float64(m.Timestamp.Nanosecond())/1e9, // 精确时间戳
		elapsedSec,
		m.MemAllocMB,
		m.MemSysMB, 
		m.MemHeapMB,
		m.GCPauseUS,
		m.NumGC,
		m.DiskReadsKB,
		m.DiskWritesKB,
		m.DiskReadOps,
		m.DiskWriteOps,
		m.IOWaitPct,
		m.NumGoroutine,
		m.CPUUsagePct,
		m.ProcReadBytes,
		m.ProcWriteBytes,
	)
	
	pm.writer.WriteString(line)
	pm.writer.Flush() // 立即刷新到磁盘
}
func (kvs *KVServer) PutInWritelessStrong(ctx context.Context, in *kvrpc.PutInWritelessStrongRequest) (*kvrpc.PutInWritelessStrongResponse, error) {
	util.DPrintf("PutInWritelessStrong %s %s", in.Key, in.Value)
	putInWritelessResponse := new(kvrpc.PutInWritelessStrongResponse)
	op := config.Log{
		Option: "Put",
		Key:    in.Key,
		Value:  in.Value,
	}
	ok := kvs.startInWritelessStrong(op, in.Vectorclock, in.Timestamp)
	if ok {
		putInWritelessResponse.Success = true
	} else {
		util.DPrintf("PutInWritelessStrong: PutInWritelessStrong Failed key=%s value=%s, Because vcFromClient < kvs.vectorclock", in.Key, in.Value)
		putInWritelessResponse.Success = false
	}
	putInWritelessResponse.Vectorclock = util.BecomeMap(kvs.vectorclock)
	return putInWritelessResponse, nil
}

func (kvs *KVServer) GetInCausal(ctx context.Context, in *kvrpc.GetInCausalRequest) (*kvrpc.GetInCausalResponse, error) {
	// util.DPrintf("GetInCausal %s", in.Key)
	getInCausalResponse := new(kvrpc.GetInCausalResponse)
	// op := config.Log{
	// 	Option: "Get",
	// 	Key:    in.Key,
	// 	Value:  "",
	// }
	// ok := kvs.startInCausal(op, in.Vectorclock, in.Timestamp)
	vcKVS, _ := kvs.vectorclock.Load(kvs.internalAddress)
	vcFromClient := util.BecomeSyncMap(in.Vectorclock)
	vcKVC, _ := vcFromClient.Load(kvs.internalAddress)

	if vcKVS.(int32) >= vcKVC.(int32) {
		/* vt, _ := kvs.db.Load(in.Key)
		if vt == nil {
			getInCausalResponse.Value = ""
			getInCausalResponse.Success = false
			return getInCausalResponse, nil
		}
		valueTimestamp := vt.(*ValueTimestamp)
		// compare timestamp
		if valueTimestamp.timestamp > in.Timestamp {
			getInCausalResponse.Value = ""
			getInCausalResponse.Success = false
		} */
		// only update the client's vectorclock if the value is newer
		getInCausalResponse.Vectorclock = util.BecomeMap(kvs.vectorclock)
		// getInCausalResponse.Value = valueTimestamp.value
		getInCausalResponse.Value = string(kvs.store.Get(in.Key))
		// val, err := kvs.memdb.Get(kvs.ctx, in.Key).Result()
		// if err != nil {
		// 	util.EPrintf(err.Error())
		// 	getInCausalResponse.Value = ""
		// 	getInCausalResponse.Success = false
		// 	return getInCausalResponse, nil
		// }
		// getInCausalResponse.Value = string(val)
		getInCausalResponse.Success = true
	} else {
		getInCausalResponse.Value = ""
		getInCausalResponse.Success = false
	}
	return getInCausalResponse, nil
}

func (kvs *KVServer) PutInCausal(ctx context.Context, in *kvrpc.PutInCausalRequest) (*kvrpc.PutInCausalResponse, error) {
	// util.DPrintf("PutInCausal %s %s", in.Key, in.Value)
	putInCausalResponse := new(kvrpc.PutInCausalResponse)
	op := config.Log{
		Option: "Put",
		Key:    in.Key,
		Value:  in.Value,
	}
	// ok := kvs.startInCausal(op, in.Vectorclock, in.Timestamp)
	kvs.startInCausal(op, in.Vectorclock, in.Timestamp)
	// if ok {
	putInCausalResponse.Success = true
	// } else {
	// 	util.DPrintf("PutInCausal: StartInCausal Failed key=%s value=%s, Because vcFromClient < kvs.vectorclock", in.Key, in.Value)
	// 	putInCausalResponse.Success = false
	// }
	putInCausalResponse.Vectorclock = util.BecomeMap(kvs.vectorclock)
	return putInCausalResponse, nil
}

// this method is used to execute the command from client with causal consistency
func (kvs *KVServer) startInWritelessCausal(command interface{}, vcFromClientArg map[string]int32, timestampFromClient int64) bool {
	vcFromClient := util.BecomeSyncMap(vcFromClientArg)
	newLog := command.(config.Log)
	util.DPrintf("Log in Start(): %v ", newLog)
	// util.DPrintf("vcFromClient in Start(): %v", vcFromClient)
	if newLog.Option == "Put" {
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
		putCounts_int := util.LoadInt(kvs.putCountsInProxy, newLog.Key)
		predictCounts_int := util.LoadInt(kvs.predictPutCounts, newLog.Key)
		if putCounts_int >= predictCounts_int {
			util.DPrintf("Sync History Puts by Prediction, predictPutCounts: %v, putCountsInProxy: %v", predictCounts_int, putCounts_int)
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
				// Version:    oldVersion + 1,
				Version: 1,
			}
			// async sending to other nodes
			for i := 0; i < len(kvs.peers); i++ {
				if kvs.peers[i] != kvs.internalAddress {
					go kvs.sendAppendEntriesInCausal(kvs.peers[i], args)
				}
			}
			kvs.putCountsInProxy.Store(newLog.Key, 0)
		}
		// update value in the db and persist
		kvs.logs = append(kvs.logs, newLog)
		// kvs.db.Store(newLog.Key, &ValueTimestamp{value: newLog.Value, timestamp: time.Now().UnixMilli(), version: oldVersion + 1})
		kvs.store.Put(newLog.Key, newLog.Value)
		// err := kvs.memdb.Set(kvs.ctx, newLog.Key, newLog.Value, 0).Err()
		// if err != nil {
		// 	panic(err)
		// }
		return true
	} else if newLog.Option == "Get" {
		vcKVS, _ := kvs.vectorclock.Load(kvs.internalAddress)
		vcKVC, _ := vcFromClient.Load(kvs.internalAddress)
		if vcKVS.(int32) >= vcKVC.(int32) {
			getCounts := util.LoadInt(kvs.getCountsInTotal, newLog.Key)
			// 该Get请求有效
			kvs.getCountsInTotal.Store(newLog.Key, getCounts+1)
			totalCounts := util.LoadInt(kvs.putCountsInTotal, newLog.Key)
			proxyCounts := util.LoadInt(kvs.putCountsInProxy, newLog.Key)
			// update predictPutCounts
			// 取平均
			kvs.predictPutCounts.Store(newLog.Key, (totalCounts+proxyCounts)/(getCounts+1))
			if proxyCounts != 0 {
				util.DPrintf("Sync History Puts by Get")
				// 同步该key之前的put
				syncLog := config.Log{
					Option: "Put",
					Key:    newLog.Key,
					Value:  string(kvs.store.Get(newLog.Key)),
				}
				ml := lattices.HybridLattice{
					Key: newLog.Key,
					Vl: lattices.ValueLattice{
						Log:         syncLog,
						VectorClock: util.BecomeMap(kvs.vectorclock),
					},
				}
				data, _ := json.Marshal(ml)
				syncReq := &causalrpc.AppendEntriesInCausalRequest{
					MapLattice: data,
					// Version:    oldVersion + 1,
					Version: 1,
				}
				for i := 0; i < len(kvs.peers); i++ {
					if kvs.peers[i] != kvs.internalAddress {
						go kvs.sendAppendEntriesInCausal(kvs.peers[i], syncReq)
					}
				}
				kvs.putCountsInProxy.Store(newLog.Key, 0)
				// kvs.putCountsByNodes.Store(newLog.Key, nil)
			}
			return true
		}
		return false
		// return util.IsUpper(kvs.vectorclock, vcFromClient)
	}
	util.DPrintf("here is Start() in Causal: log command option is false")
	return false
}

/* Writeless-Causal Consistency*/
func (kvs *KVServer) GetInWritelessCausal(ctx context.Context, in *kvrpc.GetInWritelessCausalRequest) (*kvrpc.GetInWritelessCausalResponse, error) {
	/*
		更新计数，比较预测值判断是否需要同步，更新预测值
	*/
	op := config.Log{
		Option: "Get",
		Key:    in.Key,
		Value:  "",
	}
	util.DPrintf("GetInWritelessCausal %s", in.Key)
	ok := kvs.startInWritelessCausal(op, in.Vectorclock, in.Timestamp)
	getInWritelessCausalResponse := new(kvrpc.GetInWritelessCausalResponse)
	if ok {
		getInWritelessCausalResponse.Vectorclock = util.BecomeMap(kvs.vectorclock)
		getInWritelessCausalResponse.Value = string(kvs.store.Get(in.Key))
		getInWritelessCausalResponse.Success = true
	} else {
		getInWritelessCausalResponse.Value = ""
		getInWritelessCausalResponse.Success = false
	}
	return getInWritelessCausalResponse, nil
}

func (kvs *KVServer) PutInWritelessCausal(ctx context.Context, in *kvrpc.PutInWritelessCausalRequest) (*kvrpc.PutInWritelessCausalResponse, error) {
	util.DPrintf("PutInWritelessCausal %s", in.Key)
	putInWritelessCausalResponse := new(kvrpc.PutInWritelessCausalResponse)
	/*
		更新计数， 比较预测值判断是否需要同步
	*/
	op := config.Log{
		Option: "Put",
		Key:    in.Key,
		Value:  in.Value,
	}
	proxyCounts := util.LoadInt(kvs.putCountsInProxy, in.Key)
	kvs.putCountsInProxy.Store(in.Key, proxyCounts+1)
	totalCounts := util.LoadInt(kvs.putCountsInTotal, in.Key)
	kvs.putCountsInTotal.Store(in.Key, totalCounts+1)
	// kvs.putCountsByNodes[in.Key] = append(kvs.putCountsByNodes[in.Key], kvs.internalAddress)
	ok := kvs.startInWritelessCausal(op, in.Vectorclock, in.Timestamp)
	if ok {
		putInWritelessCausalResponse.Success = true
	} else {
		util.DPrintf("PutInCausal: StartInCausal Failed key=%s value=%s, Because vcFromClient < kvs.vectorclock", in.Key, in.Value)
		putInWritelessCausalResponse.Success = false
	}
	putInWritelessCausalResponse.Vectorclock = util.BecomeMap(kvs.vectorclock)
	return putInWritelessCausalResponse, nil
}

func (kvs *KVServer) startInEventual(command interface{}, vcFromClientArg map[string]int32, timestampFromClient int64) bool {
	vcFromClient := util.BecomeSyncMap(vcFromClientArg)
	newLog := command.(config.Log)
	util.DPrintf("Log in Start(): %v ", newLog)
	if newLog.Option == "Put" {
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
		val, _ := vcFromClient.Load(kvs.internalAddress)
		kvs.vectorclock.Store(kvs.internalAddress, val.(int32)+1)
		// Sync
		ml := lattices.HybridLattice{
			Key: newLog.Key,
			Vl: lattices.ValueLattice{
				Log:         newLog,
				VectorClock: util.BecomeMap(kvs.vectorclock),
			},
		}
		data, _ := json.Marshal(ml)
		args := &eventualrpc.AppendEntriesInEventualRequest{
			MapLattice: data,
			// Version:    oldVersion + 1,
			Version: 1,
		}
		// async sending to other nodes
		for i := 0; i < len(kvs.peers); i++ {
			if kvs.peers[i] != kvs.internalAddress {
				go kvs.sendAppendEntriesInEventual(kvs.peers[i], args)
			}
		}

	} else if newLog.Option == "Get" {
		return true
	}
	util.DPrintf("here is Start() in Causal: log command option is false")
	return false
}

func (kvs *KVServer) AppendEntriesInCausal(ctx context.Context, in *causalrpc.AppendEntriesInCausalRequest) (*causalrpc.AppendEntriesInCausalResponse, error) {
	// util.DPrintf("AppendEntriesInCausal %v", in)
	appendEntriesInCausalResponse := &causalrpc.AppendEntriesInCausalResponse{}
	var mlFromOther lattices.HybridLattice
	json.Unmarshal(in.MapLattice, &mlFromOther)
	vcFromOther := util.BecomeSyncMap(mlFromOther.Vl.VectorClock)
	ok := util.IsUpper(kvs.vectorclock, vcFromOther)
	// fmt.Printf("kvs:%v,kvc:%v\n",kvs.vectorclock,vcFromOther)
	// kvs.vectorclock.Range(func(k, v interface{}) bool {
	// 	fmt.Printf("kvs:k-v:%v-%v\n",k,v)
	// 	return true
	// })
	// vcFromOther.Range(func(k, v interface{}) bool {
	// 	fmt.Printf("kvc:k-v:%v-%v\n",k,v)
	// 	return true
	// })
	if !ok {
		// if true {
		// Append the log to the local log
		kvs.logs = append(kvs.logs, mlFromOther.Vl.Log)
		// kvs.db.Store(mlFromOther.Key, &ValueTimestamp{value: mlFromOther.Vl.Log.Value, timestamp: time.Now().UnixMilli(), version: in.Version})
		kvs.store.Put(mlFromOther.Key, mlFromOther.Vl.Log.Value)
		// kvs.MergeVC(vcFromOther)
		appendEntriesInCausalResponse.Success = true
		// if len(kvs.logs) % 100 == 0 {
		// fmt.Printf("接受同步成功%v\n",len(kvs.logs))
		// }
	} else {
		// 	// Reject the log, Because of vectorclock
		appendEntriesInCausalResponse.Success = false
	}
	return appendEntriesInCausalResponse, nil
}

func (kvs *KVServer) AppendEntriesInEventual(ctx context.Context, in *eventualrpc.AppendEntriesInEventualRequest) (*eventualrpc.AppendEntriesInEventualResponse, error) {
	util.DPrintf("AppendEntriesInEventual %v", in)
	appendEntriesInEventualResponse := &eventualrpc.AppendEntriesInEventualResponse{}
	var mlFromOther lattices.HybridLattice
	json.Unmarshal(in.MapLattice, &mlFromOther)
	vcFromOther := util.BecomeSyncMap(mlFromOther.Vl.VectorClock)
	ok := util.IsUpper(kvs.vectorclock, vcFromOther)
	if !ok {
		// Append the log to the local log
		kvs.logs = append(kvs.logs, mlFromOther.Vl.Log)
		// kvs.db.Store(mlFromOther.Key, &ValueTimestamp{value: mlFromOther.Vl.Log.Value, timestamp: time.Now().UnixMilli(), version: in.Version})
		kvs.store.Put(mlFromOther.Key, mlFromOther.Vl.Log.Value)
		fmt.Println("同步成功？")
		kvs.MergeVC(vcFromOther)
		appendEntriesInEventualResponse.Success = true
	} else {
		appendEntriesInEventualResponse.Success = false
	}
	return appendEntriesInEventualResponse, nil
}

// 处理PreCommit，执行该操作
func (kvs *KVServer) PreCommit(ctx context.Context, in *strongrpc.PreCommitRequest) (*strongrpc.PreCommitResponse, error) {
	preCommitResponse := &strongrpc.PreCommitResponse{}
	var mlFromOther lattices.HybridLattice
	json.Unmarshal(in.MapLattice, &mlFromOther)
	vcFromOther := util.BecomeSyncMap(mlFromOther.Vl.VectorClock)
	ok := util.IsUpper(kvs.vectorclock, vcFromOther)
	if !ok {
		// Append the log to the local log
		kvs.logs = append(kvs.logs, mlFromOther.Vl.Log)
		// kvs.db.Store(mlFromOther.Key, &ValueTimestamp{value: mlFromOther.Vl.Log.Value, timestamp: time.Now().UnixMilli(), version: in.Version})
		kvs.store.Put(mlFromOther.Key, mlFromOther.Vl.Log.Value)
		kvs.MergeVC(vcFromOther)
		preCommitResponse.Success = true
	} else {
		preCommitResponse.Success = false
	}
	return preCommitResponse, nil
}

// 根本目的其实是为了回滚操作，但是测试环境中只是为了完成模拟两阶段通信
func (kvs *KVServer) Commit(ctx context.Context, args *strongrpc.CommitRequest) (*strongrpc.CommitResponse, error) {
	commitResponse := &strongrpc.CommitResponse{}
	return commitResponse, nil
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

func (kvs *KVServer) RegisterStrongServer(address string) {
	util.DPrintf("RegisterStrongServer: %s", address)
	for {
		lis, err := net.Listen("tcp", address)
		if err != nil {
			util.FPrintf("failed to listen: %v", err)
		}
		grpcServer := grpc.NewServer()
		// causalrpc.RegisterCAUSALServer(grpcServer, kvs)
		strongrpc.RegisterSTRONGServer(grpcServer, kvs)
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

// s0 --> other servers
func (kvs *KVServer) sendAppendEntriesInCausal(address string, args *causalrpc.AppendEntriesInCausalRequest) (*causalrpc.AppendEntriesInCausalResponse, bool) {
	// util.DPrintf("here is sendAppendEntriesInCausal() ---------> ", address)
	// 随机等待，模拟延迟
	time.Sleep(time.Millisecond * time.Duration(kvs.latency+rand.Intn(25)))
	// conn, err := grpc.Dial(address, grpc.WithInsecure(), grpc.WithBlock())
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		util.EPrintf("sendAppendEntriesInCausal did not connect: %v", err)
	}
	defer conn.Close()
	client := causalrpc.NewCAUSALClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	reply, err := client.AppendEntriesInCausal(ctx, args)
	if err != nil {
		// util.EPrintf("sendAppendEntriesInCausal could not greet: ", err, address)
		return reply, false
	}
	// if len(kvs.logs) % 100 == 0 {
	// fmt.Printf("发送同步成功%v\n",len(kvs.logs))
	// }
	return reply, true
}

func (kvs *KVServer) sendPreCommit(address string, args *strongrpc.PreCommitRequest) (*strongrpc.PreCommitResponse, bool) {
	util.DPrintf("here is sendPreCommit() ---------> ", address)
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		util.EPrintf("sendPreCommit did not connect: %v", err)
	}
	defer conn.Close()
	client := strongrpc.NewSTRONGClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	reply, err := client.PreCommit(ctx, args)
	if err != nil {
		util.EPrintf("sendPreCommit could not greet: ", err, address)
		return reply, false
	}
	return reply, true
}
func (kvs *KVServer) sendCommit(address string, args *strongrpc.CommitRequest) (*strongrpc.CommitResponse, bool) {
	util.DPrintf("here is sendCommit() ---------> ", address)
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		util.EPrintf("sendCommit did not connect: %v", err)
	}
	defer conn.Close()
	client := strongrpc.NewSTRONGClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	reply, err := client.Commit(ctx, args)
	if err != nil {
		util.EPrintf("sendCommit could not greet: ", err, address)
		return reply, false
	}
	return reply, true
}

func (kvs *KVServer) sendAppendEntriesInEventual(address string, args *eventualrpc.AppendEntriesInEventualRequest) (*eventualrpc.AppendEntriesInEventualResponse, bool) {
	// util.DPrintf("here is sendAppendEntriesInEventual() ---------> ", address)
	// 随机等待，模拟延迟
	// time.Sleep(time.Millisecond * time.Duration(kvs.latency+rand.Intn(25)))
	// conn, err := grpc.Dial(address, grpc.WithInsecure(), grpc.WithBlock())
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		util.EPrintf("sendAppendEntriesInEventual did not connect: %v", err)
	}
	defer conn.Close()
	client := eventualrpc.NewEVENTUALClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	reply, err := client.AppendEntriesInEventual(ctx, args)
	if err != nil {
		util.EPrintf("sendAppendEntriesInEventual could not greet: ", err, address)
		return reply, false
	}
	return reply, true
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
	// util.IPrintf("Make KVServer %s... ", config.Address)
	kvs := new(KVServer)
	kvs.store = new(store.Store)
	kvs.store.Init("db")
	kvs.address = address
	kvs.internalAddress = internalAddress
	kvs.peers = peers
	kvs.latency = 20
	// init vectorclock: { "192.168.10.120:30881":0, "192.168.10.121:30881":0, ... }
	for i := 0; i < len(peers); i++ {
		kvs.vectorclock.Store(peers[i], int32(0))
	}
	// value ,_ := kvs.vectorclock.Load(peers[0])
	// fmt.Println("第一个值是%v",value)
	// fmt.Printf("kvs:%v\n",kvs.vectorclock)
	// init memdb(redis)
	// redis client is a connection pool, support goroutine
	// kvs.memdb = redis.NewClient(&redis.Options{
	// 	Addr:     "localhost:6379",
	// 	Password: "", // no password set
	// 	DB:       0,  // use default DB
	// })
	kvs.ctx = context.Background()
	// 初始化map
	// kvs.putCountsByNodes = make(map[string][]string)
	// kvs.putCountsInProxy = make(map[string]int)
	// kvs.putCountsInTotal = make(map[string]int)
	// kvs.getCountsInTotal = make(map[string]int)
	// kvs.predictPutCounts = make(map[string]int)
	return kvs
}

// 初始化TCP Server
func (kvs *KVServer) RegisterTCPServer(address string) {
	util.DPrintf("RegisterTCPServer: %s", address)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Println("Error Native TCP listening", err.Error())
		return // 终止程序
	}
	// 监听并接受来自客户端的连接
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting", err.Error())
			return // 终止程序
		}
		// 处理连接
		go kvs.disributeRPC(conn)
	}
}

func (kvs *KVServer) disributeRPC(conn net.Conn) {
	for {
		buf := make([]byte, 512)
		length, err := conn.Read(buf)
		if err != nil {
			// fmt.Println("Error reading", err.Error())
			return //终止程序
		}
		fmt.Printf("%v\n", string(buf[:length]))
		var message TCPReq
		json.Unmarshal(buf[:length], &message)
		fmt.Printf("message.consistency: %v", string(message.Consistency))
		consistencyLevel := message.Consistency
		switch consistencyLevel {
		case "GetInWritelessCausal":
			key := message.Key
			vc := message.VectorClock
			ts := time.Now().UnixMicro()
			util.DPrintf("GetInWritelessCausal: %s", key)
			op := config.Log{
				Option: "Get",
				Key:    key,
				Value:  "",
			}
			ok := kvs.startInWritelessCausal(op, vc, ts)
			var tcpResp TCPResp
			if ok {
				tcpResp.VectorClock = util.BecomeMap(kvs.vectorclock)
				tcpResp.Value = string(kvs.store.Get(key))
				tcpResp.Success = true
				tcpResp.Key = key
			} else {
				tcpResp.Value = ""
				tcpResp.Success = false
			}
			res, _ := json.Marshal(tcpResp)
			conn.Write([]byte(res))
		case "PutInWritelessCausal":
			key := message.Key
			value := message.Value
			vc := message.VectorClock
			ts := time.Now().UnixMicro()
			util.DPrintf("PutInWritelessCausal: key:%s, val:%s, vc:%s, ts:%v", key, value, vc, ts)
			// conn.Write([]byte("OK"))
			op := config.Log{
				Option: message.Operation,
				Key:    key,
				Value:  value,
			}
			// 更新计数， 比较预测值判断是否需要同步
			proxyCounts := util.LoadInt(kvs.putCountsInProxy, key)
			kvs.putCountsInProxy.Store(key, proxyCounts+1)
			totalCounts := util.LoadInt(kvs.putCountsInTotal, key)
			kvs.putCountsInTotal.Store(key, totalCounts+1)
			// 以WritelessCausal一致性级别执行该请求
			ok := kvs.startInWritelessCausal(op, vc, ts)
			var tcpResp TCPResp
			tcpResp.Operation = op.Option
			tcpResp.Key = op.Key
			tcpResp.Value = op.Value
			if ok {
				tcpResp.Success = true
			} else {
				util.DPrintf("PutInWritelessCausal: StartInWritelessCausal Failed key=%s value=%s, Because vcFromClient < kvs.vectorclock", key, value)
				tcpResp.Success = false
			}
			tcpResp.VectorClock = util.BecomeMap(kvs.vectorclock)
			res, _ := json.Marshal(tcpResp)
			conn.Write([]byte(res))
		case "GetInCausal":
			key := message.Key
			vc := message.VectorClock
			ts := time.Now().UnixMicro()
			util.DPrintf("GetInCausal: %s", key)
			op := config.Log{
				Option: "Get",
				Key:    key,
				Value:  "",
			}
			ok := kvs.startInCausal(op, vc, ts)
			var tcpResp TCPResp
			if ok {
				tcpResp.VectorClock = util.BecomeMap(kvs.vectorclock)
				tcpResp.Value = string(kvs.store.Get(key))
				tcpResp.Success = true
				tcpResp.Key = key
			} else {
				tcpResp.Value = ""
				tcpResp.Success = false
			}
			res, _ := json.Marshal(tcpResp)
			conn.Write([]byte(res))
		case "PutInCausal":
			key := message.Key
			value := message.Value
			vc := message.VectorClock
			ts := time.Now().UnixMicro()
			util.DPrintf("PutInCausal: key:%s, val:%s, vc:%s, ts:%v", key, value, vc, ts)
			op := config.Log{
				Option: message.Operation,
				Key:    key,
				Value:  value,
			}
			ok := kvs.startInCausal(op, vc, ts)
			var tcpResp TCPResp
			tcpResp.Operation = op.Option
			tcpResp.Key = op.Key
			tcpResp.Value = op.Value
			if ok {
				tcpResp.Success = true
			} else {
				util.DPrintf("PutInEventual: StartInEventual Failed key=%s value=%s, Because vcFromClient < kvs.vectorclock", key, value)
				tcpResp.Success = false
			}
			tcpResp.VectorClock = util.BecomeMap(kvs.vectorclock)
			res, _ := json.Marshal(tcpResp)
			conn.Write([]byte(res))
		case "GetInEventual":
			key := message.Key
			vc := message.VectorClock
			ts := time.Now().UnixMicro()
			util.DPrintf("GetInEventual: %s", key)
			op := config.Log{
				Option: "Get",
				Key:    key,
				Value:  "",
			}
			ok := kvs.startInEventual(op, vc, ts)
			var tcpResp TCPResp
			if ok {
				tcpResp.VectorClock = util.BecomeMap(kvs.vectorclock)
				tcpResp.Value = string(kvs.store.Get(key))
				tcpResp.Success = true
				tcpResp.Key = key
			} else {
				tcpResp.Value = ""
				tcpResp.Success = false
			}
			res, _ := json.Marshal(tcpResp)
			conn.Write([]byte(res))
		case "PutInEventual":
			key := message.Key
			value := message.Value
			vc := message.VectorClock
			ts := time.Now().UnixMicro()
			util.DPrintf("PutInEventual: key:%s, val:%s, vc:%s, ts:%v", key, value, vc, ts)
			op := config.Log{
				Option: message.Operation,
				Key:    key,
				Value:  value,
			}
			ok := kvs.startInEventual(op, vc, ts)
			var tcpResp TCPResp
			tcpResp.Operation = op.Option
			tcpResp.Key = op.Key
			tcpResp.Value = op.Value
			if ok {
				tcpResp.Success = true
			} else {
				util.DPrintf("PutInEventual: StartInEventual Failed key=%s value=%s, Because vcFromClient < kvs.vectorclock", key, value)
				tcpResp.Success = false
			}
			tcpResp.VectorClock = util.BecomeMap(kvs.vectorclock)
			res, _ := json.Marshal(tcpResp)
			conn.Write([]byte(res))
		}
	}
}

func main() {
	// peers inputed by command line
	var internalAddress_arg = flag.String("internalAddress", "", "Input Your address")
	var address_arg = flag.String("address", "", "Input Your address")
	var peers_arg = flag.String("peers", "", "Input Your Peers")
	// var tcpAddress_arg = flag.String("tcpAddress", "", "Input Your TCP address")
	flag.Parse()
	internalAddress := *internalAddress_arg
	// tcpAddress := *tcpAddress_arg
	address := *address_arg
	peers := strings.Split(*peers_arg, ",")
	kvs := MakeKVServer(address, internalAddress, peers)
	go kvs.RegisterKVServer(kvs.address)
	go kvs.RegisterCausalServer(kvs.internalAddress)
	// go kvs.RegisterKVServer("0.0.0.0:8888")
	// go kvs.RegisterCausalServer("0.0.0.0:88881")
	// go kvs.RegisterStrongServer(kvs.internalAddress + "1")
	// go kvs.RegisterTCPServer(tcpAddress)
	// log.Println(http.ListenAndServe(":6060", nil))
	// server run for 20min
	// 这就是自己修改grpc线程池option参数的做法
	DesignOptions := pool.Options{
		Dial:                 pool.Dial,
		MaxIdle:              150,
		MaxActive:            400,
		MaxConcurrentStreams: 150,
		Reuse:                true,
	}
	// 根据servers的地址，创建了一一对应server地址的grpc连接池
	for i := 0; i < len(peers); i++ {
		peers_single := []string{peers[i]}
		p, err := pool.New(peers_single, DesignOptions)
		if err != nil {
			util.EPrintf("failed to new pool: %v", err)
		}
		// grpc连接池组
		kvs.pools = append(kvs.pools, p)
	}
	defer func() {
		for _, pool := range kvs.pools {
			pool.Close()
		}
	}()
	// 监控资源
	// monitor, _ := NewPerformanceMonitor("performance_metrics.csv", 500)
	// monitor.Start()
	// defer monitor.Stop()
	time.Sleep(time.Second * 1200000)
}

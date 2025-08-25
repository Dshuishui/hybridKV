package util

import (
	"encoding/csv"
	"log"
	"os"
	"strconv"
	"sync"
)

// Debugging
const Debug = true

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug {
		log.SetPrefix("[Debug] ")
		log.SetFlags(log.Ldate | log.Ltime)
		log.Printf(format, a...)
	}
	return
}

func EPrintf(format string, a ...interface{}) (n int, err error) {
	log.SetPrefix("[Error] ")
	log.SetFlags(log.Ldate | log.Ltime)
	log.Printf(format, a...)
	return
}

func IPrintf(format string, a ...interface{}) (n int, err error) {
	log.SetPrefix("[Info] ")
	log.SetFlags(log.Ldate | log.Ltime)
	log.Printf(format, a...)
	return
}

func FPrintf(format string, a ...interface{}) (n int, err error) {
	log.SetPrefix("[Fatalf] ")
	log.SetFlags(log.Ldate | log.Ltime)
	log.Printf(format, a...)
	return
}

/* csv读取 */
func ReadCsv(filepath string) (writeCounts []float64) {
	// 打开 CSV 文件
	file, err := os.Open(filepath)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	// 创建 CSV 读取器
	reader := csv.NewReader(file)

	// 读取CSV数据
	rows, err := reader.ReadAll()
	if err != nil {
		log.Fatal(err)
	}

	// 只处理前1000行
	if len(rows) > 1000 {
		rows = rows[:1000]
	}

	writeCounts = make([]float64, len(rows)-1)

	// 读取第二列数据
	for j := 1; j < len(rows) && j <= 1000; j++ {
		writeCounts[j-1], err = strconv.ParseFloat(rows[j][1], 64)
		if err != nil {
			FPrintf(err.Error())
		}
	}
	return writeCounts
}

/* 数组写入csv */
func WriteCsv(filepath string, spentTimeArr []int) {
	file, err := os.Create(filepath)
	if err != nil {
		FPrintf(err.Error())
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	for _, value := range spentTimeArr {
		err := writer.Write([]string{strconv.Itoa(value)})
		if err != nil {
			FPrintf(err.Error())
		}
	}
}

/*
sync.Map 相关的函数
*/
func Len(vc sync.Map) int {
	count := 0
	vc.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}
func BecomeMap(vc sync.Map) map[string]int32 {
	res := map[string]int32{}
	vc.Range(func(key, value any) bool {
		res[key.(string)] = value.(int32)
		return true
	})
	return res
}
func BecomeSyncMap(argMap map[string]int32) sync.Map {
	var res sync.Map
	for key, value := range argMap {
		res.Store(key, value)
	}
	return res
}

// 判断vectorClock是否更大（key都有，并且value>=other.value）
func IsUpper(vectorClock sync.Map, arg_vc sync.Map) bool {
	// DPrintf("IsUpper(): vectorClock: %v, arg_vc: %v", BecomeMap(vectorClock), BecomeMap(arg_vc))
	if Len(arg_vc) == 0 {
		return true
	}
	if Len(vectorClock) < Len(arg_vc) {
		DPrintf("vectorClock's length is shorter")
		return false
	} else {
		res := true
		vectorClock.Range(func(k, v interface{}) bool {
			value, ok := arg_vc.Load(k)
			if ok {
				if v.(int32) >= value.(int32) {
					return true
				} else {
					res = false
					// 遍历终止
					return false
				}
			}
			// vectorClock中没有该key
			res = false
			// 遍历终止
			return false
		})
		// res=true
		return res
	}
}

func LoadInt(counts sync.Map, key string) int {
	val, exist := counts.Load(key)
	if exist == false {
		return 0
	}
	return val.(int)
}

/* Map相关 */
func MakeMap(addresses []string) map[string]int32 {
	res := make(map[string]int32)
	for _, address := range addresses {
		res[address+"1"] = 0
	}
	return res
}

package store

import (
	"runtime/debug"

	"github.com/JasonLou99/Hybrid_KV_Store/util"

	"github.com/coocood/freecache"
)

type Store struct {
	// path string
	// db *leveldb.DB
	db *freecache.Cache
}

func (p *Store) Init(path string) {

	// LevelDB
	// var err error
	// //数据存储路径和一些初始文件
	// p.db, err = leveldb.OpenFile(path, nil)
	// if err != nil {
	// 	util.EPrintf("Open db failed, err: %s", err)
	// }

	// FreeCache
	cacheSize := 100 * 1024 * 1024
	p.db = freecache.NewCache(cacheSize)
	debug.SetGCPercent(20)

}

func (p *Store) Put(key string, value string) {
	//leveldb
	/* 	err := p.db.Put([]byte(key), []byte(value), nil)
	   	if err != nil {
	   		util.EPrintf("Put key %s value %s failed, err: %s", key, value, err)
	   	} */

	//freecache
	p.db.Set([]byte(key), []byte(value), 60)
}

func (p *Store) Get(key string) []byte {
	/* 	value, err := p.db.Get([]byte(key), nil)
	   	if err != nil {
	   		util.EPrintf("Get key %s failed, err: %s", key, err)
	   		return nil
	   	}
	   	return value */

	//freecache
	value, err := p.db.Get([]byte(key))
	if err != nil {
		util.DPrintf("执行get操作成功，但目前还未put对应的key：%v", key)
		return nil
	}
	return value
}

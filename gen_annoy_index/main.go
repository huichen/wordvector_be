package main

import (
	"annoyindex"
	"log"

	"github.com/huichen/wordvector_be/util"

	"github.com/syndtr/goleveldb/leveldb"
)

const (
	dimVec = 200
	numTrees = 10
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Llongfile)

	// 打开 leveldb，用于读取 wordvector 数据
	dbWordVector, err := leveldb.OpenFile("../data/tencent_embedding_wordvector.db", nil)
	if err != nil {
		log.Panic(err)
	}
	defer dbWordVector.Close()

	// 用于保存keyword->index
	dbIndexToKeywordToIndex, err := leveldb.OpenFile("../data/tencent_embedding_keyword_to_index.db", nil)
	if err != nil {
		log.Panic(err)
	}
	defer dbIndexToKeywordToIndex.Close()

	// 用于保存index->keyword
	dbIndexToKeyword, err := leveldb.OpenFile("../data/tencent_embedding_index_to_keyword.db", nil)
	if err != nil {
		log.Panic(err)
	}
	defer dbIndexToKeyword.Close()

	log.Printf("start loading data")
	count := 0
	t := annoyindex.NewAnnoyIndexAngular(dimVec)
	iter := dbWordVector.NewIterator(nil, nil)
	for iter.Next() {
		value := iter.Value()
		if count%1000000 == 0 {
			log.Printf("#records loaded = %d", count)
		}
		v := []float32{}
		for i := 0; i < dimVec; i++ {
			e := util.Float32frombytes(value[i*4 : (i+1)*4])
			v = append(v, e)
		}
		t.AddItem(count, v)
		err = dbIndexToKeywordToIndex.Put(iter.Key(), util.Uint32bytes(uint32(count)), nil)
		if err != nil {
			log.Panic(err)
		}

		err = dbIndexToKeyword.Put(util.Uint32bytes(uint32(count)), iter.Key(), nil)
		if err != nil {
			log.Panic(err)
		}
		count++
	}
	iter.Release()
	err = iter.Error()
	if err != nil {
		log.Panic(err)
	}
	log.Printf("finished loading data")

	log.Printf("start building")
	t.Build(numTrees)  // 更多的树意味着更高精度，但建造时间也更长
	log.Printf("finished building")

	t.Save("../data/tencent_embedding.ann")
	log.Printf("finished saving")
}

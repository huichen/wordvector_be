package main

import (
	"bufio"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/huichen/wordvector_be/util"
	"github.com/syndtr/goleveldb/leveldb"
)

func main() {
	db, err := leveldb.OpenFile("../data/tencent_embedding_wordvector.db", nil)
	defer db.Close()

	file, err := os.Open("../data/Tencent_AILab_ChineseEmbedding.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	var total int64
	var dim int
	for scanner.Scan() {
		lineCount++
		if lineCount == 1 {
			fields := strings.Split(scanner.Text(), " ")
			total, err = strconv.ParseInt(fields[0], 10, 0)
			if err != nil {
				log.Fatal(err)
			}
			l, err := strconv.ParseInt(fields[1], 10, 0)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("vector length = %d", l)
			dim = int(l)
			continue
		}

		fields := strings.Split(scanner.Text(), " ")
		if len(fields) != 1+dim {
			log.Fatal("向量长度不匹配 %d != %d", len(fields), 1+dim)
		}

		vec := make([]byte, 4*dim)
		for i := 1; i <= dim; i++ {
			value, err := strconv.ParseFloat(fields[i], 32)
			if err != nil {
				log.Fatal(err)
			}
			bs := util.Float32bytes(float32(value))
			vec[4*(i-1)] = bs[0]
			vec[4*(i-1)+1] = bs[1]
			vec[4*(i-1)+2] = bs[2]
			vec[4*(i-1)+3] = bs[3]
		}

		err = db.Put([]byte(fields[0]), vec, nil)
		if err != nil {
			log.Panic(err)
		}

		if lineCount%1000 == 0 {
			log.Printf("已写入 %d 条记录，共 %d 条", lineCount, total)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	log.Printf("载入 wordvector 完毕")

}

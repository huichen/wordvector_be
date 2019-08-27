package main

import (
	"annoyindex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/huichen/wordvector_be/util"

	"github.com/syndtr/goleveldb/leveldb"
)

var (
	port = flag.String("port", ":3721", "")

	dbIndexToKeyword *leveldb.DB
	dbKeywordToIndex *leveldb.DB
	annoyIndex       annoyindex.AnnoyIndex
)

func main() {
	flag.Parse()
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Llongfile)

	var err error
	dbIndexToKeyword, err = leveldb.OpenFile("data/tencent_embedding_index_to_keyword.db", nil)
	if err != nil {
		log.Panic(err)
	}
	defer dbIndexToKeyword.Close()

	dbKeywordToIndex, err = leveldb.OpenFile("data/tencent_embedding_keyword_to_index.db", nil)
	if err != nil {
		log.Panic(err)
	}
	defer dbKeywordToIndex.Close()

	annoyIndex = annoyindex.NewAnnoyIndexAngular(200)
	annoyIndex.Load("data/tencent_embedding.ann")

	http.HandleFunc("/get.similar.keywords/", getSimilarKeyword)
	go func() {
		if err := http.ListenAndServe(*port, nil); err != nil {
			panic(err)
		}
	}()

	errc := make(chan error, 2)
	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT)
		errc <- fmt.Errorf("%s", <-c)
	}()
	log.Println("terminated ", <-errc)
}

func getSimilarKeyword(w http.ResponseWriter, r *http.Request) {
	key, ok := r.URL.Query()["keyword"]
	if !ok || len(key) != 1 {
		http.Error(w, "必须输入 keyword", http.StatusInternalServerError)
		return
	}
	num, ok := r.URL.Query()["num"]
	var numKeywords int
	if !ok || len(num) != 1 {
		numKeywords = 10
	} else {
		var err error
		numKeywords, err = strconv.Atoi(num[0])
		if err != nil {
			log.Printf("%s", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	id, err := dbKeywordToIndex.Get([]byte(key[0]), nil)
	if err != nil {
		log.Printf("%s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index := util.Uint32frombytes(id)

	var result []int
	annoyIndex.GetNnsByItem(int(index), numKeywords, -1, &result)
	var sim SimilarKeywordResponse
	for _, k := range result {
		keyword, err := dbIndexToKeyword.Get(util.Uint32bytes(uint32(k)), nil)
		if err != nil {
			log.Printf("%s", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		similarityScore := getCosineSimilarity(k, int(index))
		sim.Keywords = append(sim.Keywords, Keyword{
			Word:       string(keyword),
			Similarity: similarityScore,
		})
	}

	data, err := json.Marshal(sim)
	if err != nil {
		log.Printf("%s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(200)
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

type SimilarKeywordResponse struct {
	Keywords []Keyword
}

type Keyword struct {
	Word       string
	Similarity float32
}

func getCosineSimilarity(i, j int) float32 {
	var vec1, vec2 []float32
	annoyIndex.GetItem(i, &vec1)
	annoyIndex.GetItem(j, &vec2)

	var a, b, c float32
	for id, v := range vec1 {
		a = a + v*vec2[id]
		b = b + v*v
		c = c + vec2[id]*vec2[id]
	}

	return a / float32(math.Sqrt(float64(b))*math.Sqrt(float64(c)))
}

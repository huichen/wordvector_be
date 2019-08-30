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

	"github.com/huichen/sego"
	"github.com/huichen/wordvector_be/util"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	vecDim  = 200
	kSearch = 10000

	defaultNumReturnKeywords = 10
	maxNumReturnKeywords     = 100
)

var (
	port           = flag.String("port", ":3721", "http 服务端口")
	httpPathPrefix = flag.String("http_path_prefix", "", "")
	dict           = flag.String("dict", "", "sego 词典，从 github.com/huichen/sego/data/dictionary.txt 下载")

	dbIndexToKeyword *leveldb.DB
	dbKeywordToIndex *leveldb.DB
	annoyIndex       annoyindex.AnnoyIndex
	segmenter        sego.Segmenter
)

func main() {
	flag.Parse()
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Llongfile)
	if *dict != "" {
		segmenter.LoadDictionary(*dict)
	}

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

	annoyIndex = annoyindex.NewAnnoyIndexAngular(vecDim)
	annoyIndex.Load("data/tencent_embedding.ann")

	http.HandleFunc(fmt.Sprintf("%s/get.similar.keywords/", *httpPathPrefix), getSimilarKeyword)
	http.HandleFunc(fmt.Sprintf("%s/get.similar.keywords.from.vector/", *httpPathPrefix), getSimilarKeywordFromVector)
	http.HandleFunc(fmt.Sprintf("%s/get.word.vector/", *httpPathPrefix), getWordVector)
	http.HandleFunc(fmt.Sprintf("%s/get.similarity.score/", *httpPathPrefix), getSimilarityScore)
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

/*
	从一个或者多个关键词找相似词
	HTTP 请求参数
	/get.similar.keywords/?keyword=xxx&num=yyy
	支持多个 keyword 参数（词向量之和），num不指定的话默认10个，比如
	/get.similar.keywords/?keyword=xxx&keyword=yyy&keyword=zzz

	特殊用法：当 keyword 只有一个且是词向量表中没有的短语时，并且 --dict 参数载入了一个
	词典，则将改 keyword 分词之后求多个分词的词向量之和的相似词

*/

type SimilarKeywordResponse struct {
	Keywords []Keyword `json:"keywords"`
}

type Keyword struct {
	Word       string  `json:"word"`
	Similarity float32 `json:"similarity"`
}

func getSimilarKeyword(w http.ResponseWriter, r *http.Request) {
	key, ok := r.URL.Query()["keyword"]
	if !ok || len(key) == 0 {
		http.Error(w, "必须输入 keyword", http.StatusInternalServerError)
		return
	}
	num, ok := r.URL.Query()["num"]
	var numKeywords int
	if !ok || len(num) != 1 {
		numKeywords = defaultNumReturnKeywords
	} else {
		var err error
		numKeywords, err = strconv.Atoi(num[0])
		if err != nil {
			log.Printf("%s", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if numKeywords <= 0 {
		numKeywords = defaultNumReturnKeywords
	} else if numKeywords > maxNumReturnKeywords {
		numKeywords = maxNumReturnKeywords
	}

	wordVec := make([]float32, vecDim)
	_, err := dbKeywordToIndex.Get([]byte(key[0]), nil)
	if err != nil {
		if len(key) == 1 && *dict != "" {
			// 只有一个关键词，且不出现在向量词表的特殊情况
			segments := segmenter.Segment([]byte(key[0]))
			key = sego.SegmentsToSlice(segments, false)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	validKeywords := 0
	for _, k := range key {
		id, err := dbKeywordToIndex.Get([]byte(k), nil)
		if err != nil {
			continue
		}
		validKeywords++
		index := util.Uint32frombytes(id)
		var wv []float32
		annoyIndex.GetItem(int(index), &wv)
		for i, v := range wv {
			wordVec[i] = wordVec[i] + v
		}
	}
	if validKeywords == 0 {
		http.Error(w, "没有找到匹配关键词", http.StatusInternalServerError)
		return
	}

	var result []int
	annoyIndex.GetNnsByVector(wordVec, numKeywords, kSearch, &result)
	var sim SimilarKeywordResponse
	for _, k := range result {
		keyword, err := dbIndexToKeyword.Get(util.Uint32bytes(uint32(k)), nil)
		if err != nil {
			log.Printf("%s", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		similarityScore := getCosineSimilarityByVector(wordVec, k)
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(data)
}

/*
	从一个或者多个关键词找相似词
	HTTP POST 请求参数
	/get.similar.keywords.from.vector/
	body 是 SimilarKeywordFromVectorRequest 结构体的 json
*/

type SimilarKeywordFromVectorRequest struct {
	NumKeywords int       `json:"numKeywords"`
	Vector      []float32 `json:"vector"`
}

func getSimilarKeywordFromVector(w http.ResponseWriter, r *http.Request) {
	var req SimilarKeywordFromVectorRequest
	if r.Body == nil {
		http.Error(w, "Please send a request body", 400)
		return
	}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	wordVec := req.Vector
	if len(wordVec) != vecDim {
		http.Error(w, "vector 维度不匹配", http.StatusInternalServerError)
		return
	}

	numKeywords := req.NumKeywords
	if numKeywords <= 0 {
		numKeywords = defaultNumReturnKeywords
	} else if numKeywords > maxNumReturnKeywords {
		numKeywords = maxNumReturnKeywords
	}

	var result []int
	annoyIndex.GetNnsByVector(wordVec, numKeywords, kSearch, &result)
	var sim SimilarKeywordResponse
	for _, k := range result {
		keyword, err := dbIndexToKeyword.Get(util.Uint32bytes(uint32(k)), nil)
		if err != nil {
			log.Printf("%s", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		similarityScore := getCosineSimilarityByVector(wordVec, k)
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(data)
}

/*
	返回一个或者多个关键词的词向量
	HTTP 请求参数
	/get.word.vector/?keyword=xxx
	支持多个 keyword 参数（词向量之和），比如
	/get.similar.keywords/?keyword=xxx&keyword=yyy&keyword=zzz

*/

type WordVectorResponse struct {
	Vector []float32 `json:"vector"`
}

func getWordVector(w http.ResponseWriter, r *http.Request) {
	key, ok := r.URL.Query()["keyword"]
	if !ok || len(key) == 0 {
		http.Error(w, "必须输入 keyword", http.StatusInternalServerError)
		return
	}
	wordVec := make([]float32, vecDim)
	for _, k := range key {
		id, err := dbKeywordToIndex.Get([]byte(k), nil)
		if err != nil {
			log.Printf("%s", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		index := util.Uint32frombytes(id)
		var wv []float32
		annoyIndex.GetItem(int(index), &wv)
		for i, v := range wv {
			wordVec[i] = wordVec[i] + v
		}
	}

	var resp WordVectorResponse
	resp.Vector = wordVec
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("%s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

/*
	计算两个词的相似度
	HTTP 请求参数
	/get.similarity.score/?keyword1=xxx&keyword2=yyy
*/

type SimilarityScoreResponse struct {
	Score float32 `json:"score"`
}

func getSimilarityScore(w http.ResponseWriter, r *http.Request) {
	key1, ok := r.URL.Query()["keyword1"]
	if !ok || len(key1) != 1 {
		http.Error(w, "必须输入 keyword", http.StatusInternalServerError)
		return
	}
	id1, err := dbKeywordToIndex.Get([]byte(key1[0]), nil)
	if err != nil {
		log.Printf("%s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index1 := util.Uint32frombytes(id1)

	key2, ok := r.URL.Query()["keyword2"]
	if !ok || len(key2) != 1 {
		http.Error(w, "必须输入 keyword", http.StatusInternalServerError)
		return
	}
	id2, err := dbKeywordToIndex.Get([]byte(key2[0]), nil)
	if err != nil {
		log.Printf("%s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index2 := util.Uint32frombytes(id2)

	var resp SimilarityScoreResponse
	resp.Score = getCosineSimilarity(int(index1), int(index2))
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("%s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(data)
}

func getCosineSimilarity(i, j int) float32 {
	var vec []float32
	annoyIndex.GetItem(i, &vec)

	return getCosineSimilarityByVector(vec, j)
}

func getCosineSimilarityByVector(vec []float32, j int) float32 {
	var vec2 []float32
	annoyIndex.GetItem(j, &vec2)

	var a, b, c float32
	for id, v := range vec {
		a = a + v*vec2[id]
		b = b + v*v
		c = c + vec2[id]*vec2[id]
	}

	return a / float32(math.Sqrt(float64(b))*math.Sqrt(float64(c)))
}

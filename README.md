# wordvector_be

这个项目用 go 语言实现了一个 HTTP 服务，使用腾讯 800 万词的 [word vector 模型](https://ai.tencent.com/ailab/zh/news/detial?id=22) 得到相似关键词和关键词的cosine similarity。索引使用了 spotify 的 [annoy](https://github.com/spotify/annoy) 引擎。

## 安装

一、首先安装 annoy 的 golang 包，参照 [这个文档](https://github.com/spotify/annoy/blob/master/README_GO.rst)，不需要执行所有步骤，只要执行下面命令

```
swig -go -intgosize 64 -cgo -c++ src/annoygomodule.i
mkdir -p $GOPATH/src/annoyindex
cp src/annoygomodule_wrap.cxx src/annoyindex.go \
  src/annoygomodule.h src/annoylib.h src/kissrandom.h test/annoy_test.go $GOPATH/src/annoyindex
cd $GOPATH/src/annoyindex
```

二、然后下载腾讯的模型文件，建议使用 aria2c，进入本项目目录后

```
go get github.com/huichen/wordvector_be
cd $GOPATH/src/github.com/huichen/wordvector_be
mkdir data
aria2c -c https://ai.tencent.com/ailab/nlp/data/Tencent_AILab_ChineseEmbedding.tar.gz
tar zxvf https://ai.tencent.com/ailab/nlp/data/Tencent_AILab_ChineseEmbedding.tar.gz
```

三、将腾讯的 txt 模型文件导出为 leveldb 格式的数据库，进入 gen_wordvector_leveldb 后执行

```
go run main.go
```

生成的数据库在 data/tencent_embedding_wordvector.db 目录下

四、创建 annoy 索引文件和 metadata 数据库，进入 gen_annoy_index 目录，执行

```
go run main.go
```

不到 30 分钟后，索引文件生成在 data/tencent_embedding.ann。annoy 索引的 key 是整数 id，不包括关键词和 id 之间的映射关系，这个关系放在了 data/tencent_embedding_index_to_keyword.db 和 data/tencent_embedding_keyword_to_index.db 两个 leveldb 数据库备用。

## 使用

所有包和数据文件准备好之后，就可以启动服务了：

```
go build
./wordvector_be
```

在浏览器打开 http://localhost:3721/get.similar.keywords/?keyword=美白&num=20 ，返回如下，word 字段是关键词，similarity 是关键词词向量之间的 consine similarity，约接近 1 越相似。

```
{
  "keywords": [
    {
      "word": "皮肤补水",
      "similarity": 1
    },
    {
      "word": "肌肤补水",
      "similarity": 0.9340592
    },
    {
      "word": "补水",
      "similarity": 0.8877853
    },
    {
      "word": "补水保湿",
      "similarity": 0.8768645
    },
    {
      "word": "脸部补水",
      "similarity": 0.86291945
    },
...
```


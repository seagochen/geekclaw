// Package utils 提供通用的可复用算法。
// 本文件实现了通用的 BM25 搜索引擎。
//
// 用法：
//
//	type MyDoc struct { ID string; Body string }
//
//	corpus := []MyDoc{...}
//	engine := bm25.New(corpus, func(d MyDoc) string {
//	    return d.ID + " " + d.Body
//	})
//	results := engine.Search("my query", 5)
package utils

import (
	"math"
	"sort"
	"strings"
)

// ── 调优默认值 ───────────────────────────────────────────────────────────

const (
	// DefaultBM25K1 是词频饱和因子（典型范围 1.2-2.0）。
	// 值越高，重复词项的权重越大。
	DefaultBM25K1 = 1.2

	// DefaultBM25B 是文档长度归一化因子（0 = 无归一化，1 = 完全归一化）。
	DefaultBM25B = 0.75
)

// BM25Engine 是基于通用语料库的查询时 BM25 搜索引擎。
// T 是文档类型；调用者提供 TextFunc 从每个文档中提取可搜索文本。
//
// 引擎在查询之间无状态：无缓存、无失效逻辑。
// 所有索引工作在每次 Search() 调用中执行，适合
// 频繁变化的语料库。
type BM25Engine[T any] struct {
	corpus   []T
	textFunc func(T) string
	k1       float64
	b        float64
}

// BM25Option 是配置 BM25Engine 的函数选项。
type BM25Option func(*bm25Config)

type bm25Config struct {
	k1 float64
	b  float64
}

// WithK1 覆盖词频饱和常数（默认 1.2）。
func WithK1(k1 float64) BM25Option {
	return func(c *bm25Config) { c.k1 = k1 }
}

// WithB 覆盖文档长度归一化因子（默认 0.75）。
func WithB(b float64) BM25Option {
	return func(c *bm25Config) { c.b = b }
}

// NewBM25Engine 为给定语料库创建 BM25Engine。
//
//   - corpus：任意类型 T 的文档切片。
//   - textFunc：返回文档可搜索文本的函数。
//   - opts：可选的调优参数（WithK1、WithB）。
//
// 语料库切片被引用而非复制。调用者不应与 Search() 并发修改它。
func NewBM25Engine[T any](corpus []T, textFunc func(T) string, opts ...BM25Option) *BM25Engine[T] {
	cfg := bm25Config{k1: DefaultBM25K1, b: DefaultBM25B}
	for _, o := range opts {
		o(&cfg)
	}
	return &BM25Engine[T]{
		corpus:   corpus,
		textFunc: textFunc,
		k1:       cfg.k1,
		b:        cfg.b,
	}
}

// BM25Result 是 Search 调用返回的单个排序结果。
type BM25Result[T any] struct {
	Document T
	Score    float32
}

// Search 对语料库进行查询排序并返回 top-k 结果。
// 无匹配时返回空切片（非 nil）。
//
// 复杂度：索引 O(N*L) + 评分 O(|Q|*avgPostingLen)，
// 其中 N = 语料库大小，L = 平均文档长度，Q = 查询词项。
// Top-k 提取使用固定大小最小堆：O(candidates * log k)。
func (e *BM25Engine[T]) Search(query string, topK int) []BM25Result[T] {
	if topK <= 0 {
		return []BM25Result[T]{}
	}

	queryTerms := bm25Tokenize(query)
	if len(queryTerms) == 0 {
		return []BM25Result[T]{}
	}

	N := len(e.corpus)
	if N == 0 {
		return []BM25Result[T]{}
	}

	// 步骤 1：构建每文档的词频和原始文档长度
	type docEntry struct {
		tf     map[string]uint32
		rawLen int
	}

	entries := make([]docEntry, N)
	df := make(map[string]int, 64)
	totalLen := 0

	for i, doc := range e.corpus {
		tokens := bm25Tokenize(e.textFunc(doc))
		totalLen += len(tokens)

		tf := make(map[string]uint32, len(tokens))
		for _, t := range tokens {
			tf[t]++
		}
		// df：每个词项在每个文档中只计数一次（遍历 map，键唯一）
		for t := range tf {
			df[t]++
		}

		entries[i] = docEntry{tf: tf, rawLen: len(tokens)}
	}

	avgDocLen := float64(totalLen) / float64(N)

	// 步骤 2：预计算 IDF 和每文档长度归一化
	// IDF（Robertson 平滑）：log( (N - df(t) + 0.5) / (df(t) + 0.5) + 1 )
	idf := make(map[string]float32, len(df))
	for term, freq := range df {
		idf[term] = float32(math.Log(
			(float64(N)-float64(freq)+0.5)/(float64(freq)+0.5) + 1,
		))
	}

	// docLenNorm[i] = k1 * (1 - b + b * |doc_i| / avgDocLen)
	// 使用 float32 存储 — 对排序精度足够。
	docLenNorm := make([]float32, N)
	for i, entry := range entries {
		docLenNorm[i] = float32(e.k1 * (1 - e.b + e.b*float64(entry.rawLen)/avgDocLen))
	}

	// 步骤 3：构建倒排索引（倒排列表）
	// 直接遍历 tf map — map 键已经唯一，无需去重集合。
	posting := make(map[string][]int32, len(df))
	for i, entry := range entries {
		for term := range entry.tf {
			posting[term] = append(posting[term], int32(i))
		}
	}

	// 步骤 4：通过倒排列表评分
	// 对查询词项去重，避免对同一词项双重加权。
	unique := bm25Dedupe(queryTerms)

	scores := make(map[int32]float32)
	for _, term := range unique {
		termIDF, ok := idf[term]
		if !ok {
			continue // 词项不在词汇表中 -> 零贡献
		}
		for _, docID := range posting[term] {
			freq := float32(entries[docID].tf[term])
			// TF_norm = freq * (k1+1) / (freq + docLenNorm)
			tfNorm := freq * float32(e.k1+1) / (freq + docLenNorm[docID])
			scores[docID] += termIDF * tfNorm
		}
	}

	if len(scores) == 0 {
		return []BM25Result[T]{}
	}

	// 步骤 5：通过固定大小最小堆获取 Top-K
	heap := make([]bm25ScoredDoc, 0, topK)

	for docID, sc := range scores {
		switch {
		case len(heap) < topK:
			heap = append(heap, bm25ScoredDoc{docID: docID, score: sc})
			if len(heap) == topK {
				bm25MinHeapify(heap)
			}
		case sc > heap[0].score:
			heap[0] = bm25ScoredDoc{docID: docID, score: sc}
			bm25SiftDown(heap, 0)
		}
	}

	sort.Slice(heap, func(i, j int) bool { return heap[i].score > heap[j].score })

	out := make([]BM25Result[T], len(heap))
	for i, h := range heap {
		out[i] = BM25Result[T]{
			Document: e.corpus[h.docID],
			Score:    h.score,
		}
	}
	return out
}

// bm25Tokenize 将字符串分词为小写词项，去除边缘标点。
func bm25Tokenize(s string) []string {
	raw := strings.Fields(strings.ToLower(s))
	out := raw[:0] // 复用底层数组以避免额外分配
	for _, t := range raw {
		t = strings.Trim(t, ".,;:!?\"'()/\\-_")
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// bm25Dedupe 返回去除重复词项的新切片，保留首次出现的顺序。
func bm25Dedupe(tokens []string) []string {
	seen := make(map[string]struct{}, len(tokens))
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			out = append(out, t)
		}
	}
	return out
}

// bm25ScoredDoc 表示带分数的文档。
type bm25ScoredDoc struct {
	docID int32
	score float32
}

// bm25MinHeapify 使用 Floyd 算法原地构建最小堆：O(k)。
func bm25MinHeapify(h []bm25ScoredDoc) {
	for i := len(h)/2 - 1; i >= 0; i-- {
		bm25SiftDown(h, i)
	}
}

// bm25SiftDown 从节点 i 开始恢复最小堆属性：O(log k)。
func bm25SiftDown(h []bm25ScoredDoc, i int) {
	n := len(h)
	for {
		smallest := i
		l, r := 2*i+1, 2*i+2
		if l < n && h[l].score < h[smallest].score {
			smallest = l
		}
		if r < n && h[r].score < h[smallest].score {
			smallest = r
		}
		if smallest == i {
			break
		}
		h[i], h[smallest] = h[smallest], h[i]
		i = smallest
	}
}

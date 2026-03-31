package utils

import (
	"reflect"
	"testing"
)

// testDoc 是用于测试的通用结构体。
type testDoc struct {
	ID   int
	Text string
}

func extractText(d testDoc) string {
	return d.Text
}

func TestBM25Search_EdgeCases(t *testing.T) {
	corpus := []testDoc{
		{1, "hello world"},
		{2, "foo bar"},
	}
	engine := NewBM25Engine(corpus, extractText)

	tests := []struct {
		name  string
		query string
		topK  int
	}{
		{"Zero topK", "hello", 0},
		{"Negative topK", "hello", -1},
		{"Empty query", "", 5},
		{"Query with only punctuation", "...,,,!!!", 5},
		{"No matches found", "golang", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := engine.Search(tt.query, tt.topK)
			if len(results) != 0 {
				t.Errorf("expected 0 results, got %d", len(results))
			}
			// 确保返回值不为 nil，而是空切片
			if results == nil {
				t.Errorf("expected empty slice, got nil")
			}
		})
	}
}

func TestBM25Search_EmptyCorpus(t *testing.T) {
	engine := NewBM25Engine([]testDoc{}, extractText)
	results := engine.Search("hello", 5)
	if len(results) != 0 || results == nil {
		t.Errorf("expected empty slice from empty corpus, got %v", results)
	}
}

func TestBM25Search_RankingLogic(t *testing.T) {
	corpus := []testDoc{
		{1, "the quick brown fox jumps over the lazy dog"},
		{2, "quick fox"},
		{3, "quick quick quick fox"}, // 词频（TF）极高
		{4, "completely irrelevant document here"},
	}
	engine := NewBM25Engine(corpus, extractText)

	t.Run("Term Frequency (TF) boosts score", func(t *testing.T) {
		results := engine.Search("quick", 5)
		if len(results) < 3 {
			t.Fatalf("expected at least 3 results, got %d", len(results))
		}
		// 文档 3 中 "quick" 出现 3 次，应排在文档 2 前面
		if results[0].Document.ID != 3 {
			t.Errorf("expected doc 3 to rank first due to high TF, got doc %d", results[0].Document.ID)
		}
	})

	t.Run("Document Length penalty", func(t *testing.T) {
		results := engine.Search("fox", 5)
		if len(results) < 3 {
			t.Fatalf("expected at least 3 results, got %d", len(results))
		}
		// 文档 2（"quick fox"）远短于文档 1（"the quick brown fox..."），
		// 因此在 "fox" 词频相同（均为 1 次）的情况下，文档 2 得分更高。
		if results[0].Document.ID != 2 {
			t.Errorf("expected doc 2 to rank first due to shorter length, got doc %d", results[0].Document.ID)
		}
	})

	t.Run("TopK limits results", func(t *testing.T) {
		results := engine.Search("quick", 2)
		if len(results) != 2 {
			t.Errorf("expected exactly 2 results, got %d", len(results))
		}
	})
}

func TestBM25Tokenize(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"Hello World", []string{"hello", "world"}},
		{"  spaces   everywhere  ", []string{"spaces", "everywhere"}},
		{"punctuation... test!!!", []string{"punctuation", "test"}},
		{"(parentheses) and-hyphens", []string{"parentheses", "and-hyphens"}}, // 连字符从边缘裁剪
		{"internal-hyphen is kept", []string{"internal-hyphen", "is", "kept"}},
		{".,;?!", []string{}}, // 裁剪后变为空
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := bm25Tokenize(tt.input)
			if len(got) == 0 && len(tt.expected) == 0 {
				return // 两者均为空
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("bm25Tokenize(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBM25Dedupe(t *testing.T) {
	input := []string{"apple", "banana", "apple", "orange", "banana"}
	expected := []string{"apple", "banana", "orange"}

	got := bm25Dedupe(input)
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("bm25Dedupe() = %v, want %v", got, expected)
	}
}

func TestBM25Options(t *testing.T) {
	corpus := []testDoc{{1, "test"}}

	engine := NewBM25Engine(
		corpus,
		extractText,
		WithK1(2.5),
		WithB(0.9),
	)

	if engine.k1 != 2.5 {
		t.Errorf("expected k1 to be 2.5, got %v", engine.k1)
	}
	if engine.b != 0.9 {
		t.Errorf("expected b to be 0.9, got %v", engine.b)
	}
}

func TestBM25Search_SortingStability(t *testing.T) {
	// 确保堆排序结果严格按分数降序排列
	corpus := []testDoc{
		{1, "golang is good"},
		{2, "golang golang"},
		{3, "golang golang golang"},
		{4, "golang golang golang golang"},
	}
	engine := NewBM25Engine(corpus, extractText)
	results := engine.Search("golang", 10)

	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	// 分数应严格单调递减
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not sorted correctly: result %d score (%v) > result %d score (%v)",
				i, results[i].Score, i-1, results[i-1].Score)
		}
	}
}

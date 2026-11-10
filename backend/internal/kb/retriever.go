package kb

import (
	"context"
	"log"
	"math"
	"sort"
)

// Embedder — эмбеддинги и реранкер (API платформы: bge-m3 и реранкер). Может быть nil — тогда только BM25.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float64, error)
	Rerank(ctx context.Context, query string, docs []string, topN int) ([]int, error)
}

// Retriever — гибридный поиск: BM25 + эмбеддинги (объединение RRF) → реранкер.
type Retriever struct {
	Chunks  []Chunk
	texts   []string
	bm25    *BM25
	vectors [][]float64
	emb     Embedder
}

func NewRetriever(chunks []Chunk, emb Embedder) *Retriever {
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.DocTitle + "\n" + c.Title + "\n" + c.Text
	}
	return &Retriever{Chunks: chunks, texts: texts, bm25: NewBM25(texts), emb: emb}
}

// PrepareEmbeddings считает эмбеддинги всех фрагментов (при старте сервиса).
func (r *Retriever) PrepareEmbeddings(ctx context.Context) {
	if r.emb == nil || len(r.texts) == 0 {
		return
	}
	var all [][]float64
	for i := 0; i < len(r.texts); i += 32 {
		v, err := r.emb.Embed(ctx, r.texts[i:min(i+32, len(r.texts))])
		if err != nil {
			log.Printf("эмбеддинги недоступны (%v) — только BM25", err)
			return
		}
		all = append(all, v...)
	}
	r.vectors = all
	log.Printf("эмбеддинги фрагментов готовы: %d", len(all))
}

func (r *Retriever) Search(ctx context.Context, query string, candidates, topN int) []Hit {
	if len(r.Chunks) == 0 {
		return nil
	}
	bm := r.bm25.Scores(ExpandQuery(query))
	rrf := map[int]float64{}
	for rank, i := range argsort(bm) {
		if bm[i] > 0 {
			rrf[i] += 1 / float64(60+rank)
		}
	}
	if r.vectors != nil && r.emb != nil {
		if q, err := r.emb.Embed(ctx, []string{query}); err == nil && len(q) == 1 {
			sims := make([]float64, len(r.vectors))
			for i, v := range r.vectors {
				sims[i] = dot(q[0], v)
			}
			for rank, i := range argsort(sims)[:min(candidates, len(sims))] {
				rrf[i] += 1 / float64(60+rank)
			}
		} else if err != nil {
			log.Printf("эмбеддинг запроса не получен: %v", err)
		}
	}
	pool := make([]int, 0, len(rrf))
	for i := range rrf {
		pool = append(pool, i)
	}
	sort.SliceStable(pool, func(a, b int) bool { return rrf[pool[a]] > rrf[pool[b]] })
	pool = pool[:min(candidates, len(pool))]
	if len(pool) == 0 {
		return nil
	}
	hit := func(i int) Hit { return Hit{Chunk: r.Chunks[i], Score: rrf[i]} }
	if r.emb != nil {
		docs := make([]string, len(pool))
		for j, i := range pool {
			docs[j] = r.texts[i]
		}
		if order, err := r.emb.Rerank(ctx, query, docs, topN); err == nil && len(order) > 0 {
			var out []Hit
			for _, j := range order {
				if j >= 0 && j < len(pool) && len(out) < topN {
					out = append(out, hit(pool[j]))
				}
			}
			return out
		} else if err != nil {
			log.Printf("реранкер недоступен: %v", err)
		}
	}
	var out []Hit
	for _, i := range pool[:min(topN, len(pool))] {
		out = append(out, hit(i))
	}
	return out
}

func argsort(xs []float64) []int {
	idx := make([]int, len(xs))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return xs[idx[a]] > xs[idx[b]] })
	return idx
}

func dot(a, b []float64) float64 {
	s := 0.0
	for i := range a {
		if i < len(b) {
			s += a[i] * b[i]
		}
	}
	return s
}

// Normalize приводит вектор к единичной длине.
func Normalize(v []float64) []float64 {
	n := 0.0
	for _, x := range v {
		n += x * x
	}
	n = math.Sqrt(n)
	if n == 0 {
		return v
	}
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = x / n
	}
	return out
}

package kb

import (
	"context"
	"log"
	"math"
	"regexp"
	"sort"
	"strings"
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

// Search ищет по текущему вопросу query; dialog — предыдущие реплики пользователя (вес вдвое меньше),
// чтобы уточняющий вопрос не терял тему, а смена темы не тонула в старой.
func (r *Retriever) Search(ctx context.Context, query, dialog string, candidates, topN int) []Hit {
	if len(r.Chunks) == 0 {
		return nil
	}
	queries := []string{query}
	weights := []float64{1}
	if dialog != "" {
		queries = append(queries, dialog+" "+query)
		weights = append(weights, 0.5)
	}
	rrf := map[int]float64{}
	for qi, q := range queries {
		bm := r.bm25.Scores(ExpandQuery(q))
		for rank, i := range argsort(bm)[:min(2*candidates, len(bm))] {
			if bm[i] > 0 {
				rrf[i] += weights[qi] / float64(60+rank)
			}
		}
	}
	if r.vectors != nil && r.emb != nil {
		if qv, err := r.emb.Embed(ctx, queries); err == nil && len(qv) == len(queries) {
			for qi := range qv {
				sims := make([]float64, len(r.vectors))
				for i, v := range r.vectors {
					sims[i] = dot(qv[qi], v)
				}
				for rank, i := range argsort(sims)[:min(candidates, len(sims))] {
					rrf[i] += weights[qi] / float64(60+rank)
				}
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
		rq := query
		if dialog != "" {
			rq = query + "\nКонтекст диалога: " + dialog
		}
		if order, err := r.emb.Rerank(ctx, rq, docs, topN); err == nil && len(order) > 0 {
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

// BestInDoc — лучшие по BM25 фрагменты конкретного документа (для явных ссылок «ст. 164 НК»).
func (r *Retriever) BestInDoc(query, doc string, n int) []Hit {
	scores := r.bm25.Scores(ExpandQuery(query))
	var out []Hit
	for _, i := range argsort(scores) {
		if r.Chunks[i].Doc == doc && len(out) < n {
			out = append(out, Hit{Chunk: r.Chunks[i], Score: scores[i]})
		}
	}
	return out
}

// DocChunks — все фрагменты документа в порядке текста.
func (r *Retriever) DocChunks(doc string) []Hit {
	var out []Hit
	for _, c := range r.Chunks {
		if c.Doc == doc {
			out = append(out, Hit{Chunk: c})
		}
	}
	return out
}

var (
	artRe      = regexp.MustCompile(`(?i)(?:^|[^\pL])(?:ст\.?|стать\pL*)\s*(\d+(?:\.\d+)?)`)
	codeRe     = regexp.MustCompile(`(?i)(?:^|[^\pL])(тк|нк|коап|гк)(?:[^\pL]|$)|(\d+)\s*-?\s*фз(?:[^\pL]|$)`)
	titleArtRe = regexp.MustCompile(`Статья\s+(\d+(?:\.\d+)?)\.`)
	titleFZRe  = regexp.MustCompile(`N\s*(\d+)-ФЗ`)
	titleCode  = regexp.MustCompile(`(ТК|НК|КоАП|ГК)\s+РФ`)
)

func lawKeys(d *Document) (string, map[string]bool) {
	codes := map[string]bool{}
	for _, m := range titleFZRe.FindAllStringSubmatch(d.Title, -1) {
		codes[m[1]+"фз"] = true
	}
	for _, m := range titleCode.FindAllStringSubmatch(d.Title, -1) {
		codes[strings.ToLower(m[1])] = true
	}
	stem := d.Name
	if i := strings.Index(stem, "__"); i >= 0 {
		stem = stem[i+2:]
	}
	stem = strings.SplitN(stem, "_ст", 2)[0]
	codes[strings.ToLower(strings.ReplaceAll(stem, "-", ""))] = true
	art := ""
	if m := titleArtRe.FindStringSubmatch(d.Title); m != nil {
		art = m[1]
	}
	return art, codes
}

// ReferencedLaws — статьи, на которые вопрос ссылается явно: «ст. 164 НК», «статья 70 ТК РФ», «152-ФЗ».
func ReferencedLaws(question string, laws []*Document) []*Document {
	arts := map[string]bool{}
	for _, m := range artRe.FindAllStringSubmatch(question, -1) {
		arts[m[1]] = true
	}
	codes := map[string]bool{}
	for _, m := range codeRe.FindAllStringSubmatch(question, -1) {
		if m[1] != "" {
			codes[strings.ToLower(m[1])] = true
		} else {
			codes[m[2]+"фз"] = true
		}
	}
	var out []*Document
	for _, d := range laws {
		art, dc := lawKeys(d)
		common := false
		for c := range codes {
			common = common || dc[c]
		}
		if len(arts) > 0 {
			if arts[art] && (len(codes) == 0 || common) {
				out = append(out, d)
			}
		} else if common && !dc["тк"] && !dc["нк"] && !dc["коап"] && !dc["гк"] {
			out = append(out, d) // «по 152-ФЗ» без номера статьи — отдельный закон, а не кодекс
		}
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

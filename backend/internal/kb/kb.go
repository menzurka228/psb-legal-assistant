// Package kb — база знаний: загрузка документов, нарезка на положения и BM25-поиск.
//
// Стартовая версия поиска — лексическая. Точка расширения для команды ML/Retrieval:
// гибридный поиск (bge-m3) и реранкинг (Qwen3 Reranker) реализуются через интерфейс Searcher.
package kb

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Chunk — одно положение документа (статья, пункт, раздел).
type Chunk struct {
	ID    string `json:"id"`
	Doc   string `json:"doc"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

// Hit — найденный фрагмент с оценкой релевантности.
type Hit struct {
	Chunk
	Score float64 `json:"score"`
}

// Searcher — контракт поиска, чтобы можно было подменить BM25 на гибридный поиск.
type Searcher interface {
	Search(query string, topK int) []Hit
}

type KnowledgeBase struct {
	Documents map[string]string
	Chunks    []Chunk

	// BM25-индекс
	tf     []map[string]int
	docLen []int
	avgLen float64
	df     map[string]int
}

const (
	bm25K1      = 1.5
	bm25B       = 0.75
	maxChunkLen = 1500
)

var (
	// Крупные структурные единицы: становятся «родителем» для пунктов
	sectionRe = regexp.MustCompile(`(?i)^\s*(статья\s+\d+[\.\d]*|глава\s+\d+|раздел\s+[\divx]+)`)
	// Пункты внутри статьи: «1.», «2.3.», «п. 4»
	pointRe = regexp.MustCompile(`(?i)^\s*(\d+(\.\d+)*\.\s|п\.\s*\d+)`)
)

// Load читает все поддерживаемые файлы из dir (рекурсивно) и строит индекс.
func Load(dir string) (*KnowledgeBase, error) {
	kb := &KnowledgeBase{Documents: map[string]string{}}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		text, err := readDocument(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if strings.TrimSpace(text) == "" {
			return nil
		}
		name, _ := filepath.Rel(dir, path)
		name = filepath.ToSlash(name)
		kb.Documents[name] = text
		kb.Chunks = append(kb.Chunks, SplitIntoChunks(name, text)...)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	kb.buildIndex()
	return kb, nil
}

// FromDocuments строит базу из готовых текстов (удобно для тестов).
func FromDocuments(docs map[string]string) *KnowledgeBase {
	kb := &KnowledgeBase{Documents: docs}
	names := make([]string, 0, len(docs))
	for n := range docs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		kb.Chunks = append(kb.Chunks, SplitIntoChunks(n, docs[n])...)
	}
	kb.buildIndex()
	return kb
}

func readDocument(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".md":
		b, err := os.ReadFile(path)
		return string(b), err
	case ".docx":
		return readDocx(path)
	default:
		return "", nil // pdf и прочее — сконвертировать заранее в txt/md
	}
}

// readDocx вытаскивает текст абзацев из word/document.xml.
func readDocx(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		var sb strings.Builder
		dec := xml.NewDecoder(rc)
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "t" {
					var s string
					if err := dec.DecodeElement(&s, &t); err == nil {
						sb.WriteString(s)
					}
				} else if t.Name.Local == "tab" {
					sb.WriteString("\t")
				}
			case xml.EndElement:
				if t.Name.Local == "p" {
					sb.WriteString("\n")
				}
			}
		}
		return sb.String(), nil
	}
	return "", nil
}

// SplitIntoChunks режет документ по заголовкам положений.
func SplitIntoChunks(doc, text string) []Chunk {
	var chunks []Chunk
	title := "Начало документа"
	var buf []string
	flush := func() {
		body := []rune(strings.TrimSpace(strings.Join(buf, "\n")))
		for i := 0; i < len(body); i += maxChunkLen {
			end := min(i+maxChunkLen, len(body))
			chunks = append(chunks, Chunk{
				ID: fmt.Sprintf("%s#%d", doc, len(chunks)), Doc: doc, Title: title, Text: string(body[i:end]),
			})
		}
	}
	section := "" // текущая статья/глава — добавляется к заголовку пункта
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case sectionRe.MatchString(line):
			flush()
			section = truncate(trimmed, 120)
			title, buf = section, nil
		case pointRe.MatchString(line):
			flush()
			point := "п. " + strings.TrimSuffix(strings.Fields(trimmed)[0], ".")
			if strings.HasPrefix(strings.ToLower(trimmed), "п.") {
				point = truncate(trimmed, 40)
			}
			title, buf = point, nil
			if section != "" {
				title = section + " › " + point
			}
		}
		buf = append(buf, line)
	}
	flush()
	return chunks
}

// Tokenize: нижний регистр, буквы/цифры, грубый стемминг обрезкой до 5 символов (заменить на Snowball).
func Tokenize(s string) []string {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for i, w := range words {
		words[i] = truncate(w, 5)
	}
	return words
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func (kb *KnowledgeBase) buildIndex() {
	kb.df = map[string]int{}
	kb.tf = make([]map[string]int, len(kb.Chunks))
	kb.docLen = make([]int, len(kb.Chunks))
	total := 0
	for i, c := range kb.Chunks {
		toks := Tokenize(c.Title + " " + c.Text)
		tf := map[string]int{}
		for _, t := range toks {
			tf[t]++
		}
		for t := range tf {
			kb.df[t]++
		}
		kb.tf[i], kb.docLen[i] = tf, len(toks)
		total += len(toks)
	}
	if len(kb.Chunks) > 0 {
		kb.avgLen = float64(total) / float64(len(kb.Chunks))
	}
}

// Search возвращает topK фрагментов с ненулевым BM25-скором.
func (kb *KnowledgeBase) Search(query string, topK int) []Hit {
	n := float64(len(kb.Chunks))
	if n == 0 {
		return nil
	}
	terms := map[string]bool{}
	for _, t := range Tokenize(query) {
		terms[t] = true
	}
	var hits []Hit
	for i, tf := range kb.tf {
		score := 0.0
		for t := range terms {
			f := float64(tf[t])
			if f == 0 {
				continue
			}
			df := float64(kb.df[t])
			idf := math.Log(1 + (n-df+0.5)/(df+0.5))
			norm := f * (bm25K1 + 1) / (f + bm25K1*(1-bm25B+bm25B*float64(kb.docLen[i])/kb.avgLen))
			score += idf * norm
		}
		if score > 0 {
			hits = append(hits, Hit{Chunk: kb.Chunks[i], Score: score})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].Score > hits[b].Score })
	if len(hits) > topK {
		hits = hits[:topK]
	}
	return hits
}

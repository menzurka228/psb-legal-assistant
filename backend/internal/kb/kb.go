// Package kb — база знаний: загрузка документов, классификация (документы компании / законы),
// очистка от инъекций, нарезка на фрагменты и лексический поиск BM25.
//
// Логика повторяет submission/solution.py: при изменениях правьте обе версии.
package kb

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const (
	KindCompany = "company"
	KindLaw     = "law"
)

// Document — документ базы знаний.
type Document struct {
	Name  string `json:"name"`  // путь файла относительно базы — ключ документа
	Kind  string `json:"kind"`  // company | law
	Title string `json:"title"` // человекочитаемое название
	Text  string `json:"-"`     // очищенный текст
}

// Chunk — фрагмент документа (раздел договора, абзацы статьи закона).
type Chunk struct {
	ID       string `json:"id"`
	Doc      string `json:"doc"`      // Name документа
	DocTitle string `json:"docTitle"` // Title документа
	Kind     string `json:"kind"`
	Title    string `json:"title"` // заголовок фрагмента
	Text     string `json:"text"`
}

// Hit — найденный фрагмент с оценкой.
type Hit struct {
	Chunk
	Score float64 `json:"score"`
}

type KnowledgeBase struct {
	Docs          []*Document
	ByName        map[string]*Document
	Company       []*Document
	Laws          []*Document
	CompanyChunks []Chunk
	LawChunks     []Chunk
}

// Строки, адресованные ассистенту, — инъекции, а не положения документа (в Go \w не видит кириллицу — используем \pL).
var injectionRe = regexp.MustCompile(`(?i)(для\s+ии|ии-ассистент|ai[- ]?assistant|языков\pL*\s+модел|игнорируй|ignore\s+(all|previous)|не\s+упоминай\s+эту\s+инструкц|system\s*prompt|системн\pL*\s+промпт)`)
var lawMetaRe = regexp.MustCompile(`(?i)^(URL|Дата выгрузки):`)
var lawFileRe = regexp.MustCompile(`(ФЗ|НК|ТК|КоАП|ГК)_`)

// Load читает поддерживаемые файлы из dir (рекурсивно).
func Load(dir string) (*KnowledgeBase, error) {
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		text, err := readDocument(path)
		if err != nil {
			log.Printf("пропущен нечитаемый документ %s: %v", path, err)
			return nil
		}
		if strings.TrimSpace(text) != "" {
			name, _ := filepath.Rel(dir, path)
			files[filepath.ToSlash(name)] = text
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return FromDocuments(files), nil
}

// FromDocuments строит базу из готовых текстов: имя файла → текст.
func FromDocuments(files map[string]string) *KnowledgeBase {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	kb := &KnowledgeBase{ByName: map[string]*Document{}}
	for _, n := range names {
		d := MakeDocument(n, files[n])
		kb.Docs = append(kb.Docs, d)
		kb.ByName[n] = d
		if d.Kind == KindLaw {
			kb.Laws = append(kb.Laws, d)
			kb.LawChunks = append(kb.LawChunks, ChunkLaw(d, 1400)...)
		} else {
			kb.Company = append(kb.Company, d)
			kb.CompanyChunks = append(kb.CompanyChunks, ChunkCompany(d, 2500)...)
		}
	}
	return kb
}

// MakeDocument классифицирует документ, очищает текст и формирует название.
func MakeDocument(name, text string) *Document {
	stem := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	category, title, found := strings.Cut(stem, "__")
	if !found {
		category, title = "", stem
	}
	isLaw := strings.Contains(strings.ToLower(category), "закон") || lawFileRe.MatchString(stem)
	text = Sanitize(stem, strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n")))
	d := &Document{Name: name, Kind: KindCompany, Title: title}
	if isLaw {
		d.Kind = KindLaw
		lines := strings.Split(text, "\n")
		if len(lines) > 0 && strings.HasPrefix(strings.ToLower(lines[0]), "источник:") {
			_, src, _ := strings.Cut(lines[0], ":")
			d.Title = strings.TrimSpace(strings.Replace(src, " (с изменениями и дополнениями)", "", 1))
		}
		kept := lines[:0]
		for _, l := range lines {
			if !lawMetaRe.MatchString(strings.TrimSpace(l)) {
				kept = append(kept, l)
			}
		}
		text = strings.Join(kept, "\n")
	}
	d.Text = text
	return d
}

// Sanitize убирает строки-инъекции, адресованные ИИ.
func Sanitize(name, text string) string {
	var kept []string
	for _, line := range strings.Split(text, "\n") {
		if injectionRe.MatchString(line) {
			log.Printf("удалена строка-инъекция из «%s»: %.80s…", name, line)
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func readDocument(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".md":
		b, err := os.ReadFile(path)
		return strings.TrimPrefix(string(b), "\uFEFF"), err
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

// ───────────────────────────── нарезка ─────────────────────────────

// Раздел документа компании: «1. Предмет договора» (но не «1.1. …»).
var sectionRe = regexp.MustCompile(`^\s*\d+\.\s+\p{Lu}`)

// ChunkCompany режет документ компании по разделам верхнего уровня; шапка — отдельный фрагмент.
func ChunkCompany(d *Document, maxLen int) []Chunk {
	var chunks []Chunk
	title := "Шапка документа"
	var buf []string
	flush := func() {
		body := strings.TrimSpace(strings.Join(buf, "\n"))
		for _, part := range splitLen(body, maxLen) {
			chunks = append(chunks, Chunk{ID: fmt.Sprintf("%s#%d", d.Name, len(chunks)), Doc: d.Name, DocTitle: d.Title, Kind: d.Kind, Title: title, Text: part})
		}
	}
	for _, line := range strings.Split(d.Text, "\n") {
		if sectionRe.MatchString(line) {
			flush()
			title, buf = Truncate(strings.TrimSpace(line), 100), nil
		}
		buf = append(buf, line)
	}
	flush()
	if len(chunks) == 1 {
		chunks[0].Title = d.Title // короткий документ без разделов (письмо, записка)
	}
	return chunks
}

var pointStartRe = regexp.MustCompile(`^\d+(\.\d+)*[.)]\s`)

// ChunkLaw режет статью закона по абзацам, не разрывая абзац, фрагменты до maxLen символов.
func ChunkLaw(d *Document, maxLen int) []Chunk {
	var paras []string
	var cur []string
	for _, line := range strings.Split(d.Text, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || pointStartRe.MatchString(t) {
			if len(cur) > 0 {
				paras = append(paras, strings.Join(cur, "\n"))
				cur = nil
			}
		}
		if t != "" {
			cur = append(cur, t)
		}
	}
	if len(cur) > 0 {
		paras = append(paras, strings.Join(cur, "\n"))
	}
	var chunks []Chunk
	buf := ""
	add := func(text string) {
		first := strings.SplitN(text, "\n", 2)[0]
		chunks = append(chunks, Chunk{ID: fmt.Sprintf("%s#%d", d.Name, len(chunks)), Doc: d.Name, DocTitle: d.Title, Kind: d.Kind, Title: Truncate(first, 90), Text: text})
	}
	for _, p := range paras {
		if buf != "" && runeLen(buf)+runeLen(p) > maxLen {
			add(buf)
			buf = ""
		}
		for runeLen(p) > maxLen { // очень длинный абзац — режем по предложению
			r := []rune(p)
			cut := strings.LastIndex(string(r[:maxLen]), ". ")
			cutRunes := maxLen
			if cut > 0 && runeLen(string(r[:maxLen])[:cut]) > maxLen/2 {
				cutRunes = runeLen(string(r[:maxLen])[:cut]) + 1
			}
			add(strings.TrimSpace(buf + "\n" + string(r[:cutRunes])))
			buf, p = "", strings.TrimSpace(string(r[cutRunes:]))
		}
		if buf == "" {
			buf = p
		} else {
			buf += "\n\n" + p
		}
	}
	if strings.TrimSpace(buf) != "" {
		add(buf)
	}
	return chunks
}

func splitLen(s string, n int) []string {
	r := []rune(s)
	if len(r) == 0 {
		return nil
	}
	var out []string
	for i := 0; i < len(r); i += n {
		out = append(out, string(r[i:min(i+n, len(r))]))
	}
	return out
}

func runeLen(s string) int { return len([]rune(s)) }

// Truncate обрезает строку до n символов.
func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// ───────────────────────────── BM25 ─────────────────────────────

// Аббревиатуры в вопросах ↔ полные формулировки в законах.
var abbreviations = map[string]string{
	"ндс": "налог на добавленную стоимость", "ндфл": "налог на доходы физических лиц", "пдн": "персональные данные",
	"мрот": "минимальный размер оплаты труда", "коап": "кодекс об административных правонарушениях",
	"тк": "трудовой кодекс", "нк": "налоговый кодекс", "усн": "упрощенная система налогообложения",
	"есхн": "единый сельскохозяйственный налог",
}

// ExpandQuery добавляет расшифровки аббревиатур.
func ExpandQuery(q string) string {
	words := map[string]bool{}
	for _, w := range rawWords(q) {
		words[w] = true
	}
	extra := []string{q}
	for abbr, full := range abbreviations {
		if words[abbr] {
			extra = append(extra, full)
		}
	}
	return strings.Join(extra, " ")
}

func rawWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(strings.ReplaceAll(s, "ё", "е")), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// Tokenize: нижний регистр, буквы/цифры, слова от 2 символов, грубый стемминг обрезкой до 5 символов.
func Tokenize(s string) []string {
	var out []string
	for _, w := range rawWords(s) {
		if runeLen(w) > 1 {
			out = append(out, Truncate(w, 5))
		}
	}
	return out
}

type BM25 struct {
	tf    []map[string]int
	lens  []int
	avg   float64
	idf   map[string]float64
	k1, b float64
}

func NewBM25(texts []string) *BM25 {
	m := &BM25{k1: 1.5, b: 0.75, idf: map[string]float64{}}
	df := map[string]int{}
	total := 0
	for _, t := range texts {
		tf := map[string]int{}
		toks := Tokenize(t)
		for _, w := range toks {
			tf[w]++
		}
		for w := range tf {
			df[w]++
		}
		m.tf = append(m.tf, tf)
		m.lens = append(m.lens, len(toks))
		total += len(toks)
	}
	n := float64(len(texts))
	if n > 0 {
		m.avg = float64(total) / n
	}
	for w, f := range df {
		m.idf[w] = math.Log(1 + (n-float64(f)+0.5)/(float64(f)+0.5))
	}
	return m
}

func (m *BM25) Scores(query string) []float64 {
	terms := map[string]bool{}
	for _, t := range Tokenize(query) {
		terms[t] = true
	}
	out := make([]float64, len(m.tf))
	for i, tf := range m.tf {
		for t := range terms {
			f := float64(tf[t])
			if f == 0 {
				continue
			}
			out[i] += m.idf[t] * f * (m.k1 + 1) / (f + m.k1*(1-m.b+m.b*float64(m.lens[i])/m.avg))
		}
	}
	return out
}

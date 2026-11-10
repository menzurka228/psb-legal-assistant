// Package llm — клиент OpenAI-совместимого API платформы: /chat/completions, /embeddings, /rerank.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/menzurka228/psb-legal-assistant/backend/internal/kb"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Chatter — генерация ответа (подменяется в тестах).
type Chatter interface {
	Chat(ctx context.Context, messages []Message) (string, error)
}

type Client struct {
	BaseURL     string
	APIKey      string
	Model       string
	Temperature float64
	MaxTokens   int
	HTTP        *http.Client

	EmbedKey    string
	EmbedModel  string
	RerankKey   string
	RerankModel string

	mu     sync.Mutex
	chosen map[string]string // путь API → подошедшее имя модели
	dead   map[string]bool   // путь API → ни одно имя не подошло
}

// Варианты имён моделей: точные идентификаторы в API организаторов не опубликованы,
// поэтому перебираем типичные и запоминаем рабочее (как ModelPicker в solution.py).
func candidates(primary string, variants ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range append([]string{primary}, variants...) {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

type statusError struct {
	code int
	msg  string
}

func (e *statusError) Error() string { return e.msg }

// postModel вызывает path, подбирая имя модели среди names при ответах 400/404/422.
func (c *Client) postModel(ctx context.Context, path, key string, names []string, payload map[string]any, out any) error {
	c.mu.Lock()
	if c.chosen == nil {
		c.chosen, c.dead = map[string]string{}, map[string]bool{}
	}
	if c.dead[path] {
		c.mu.Unlock()
		return fmt.Errorf("%s: модель недоступна", path)
	}
	if name, ok := c.chosen[path]; ok { // имя уже подобрано — запросы идут параллельно
		c.mu.Unlock()
		payload["model"] = name
		return c.post(ctx, path, key, payload, out)
	}
	defer c.mu.Unlock() // подбор имени — один раз, остальные запросы ждут его окончания
	var last error
	for _, name := range names {
		payload["model"] = name
		err := c.post(ctx, path, key, payload, out)
		if err == nil {
			c.chosen[path] = name
			return nil
		}
		last = err
		var se *statusError
		if !asStatus(err, &se) || (se.code != 400 && se.code != 404 && se.code != 422) {
			return err
		}
	}
	c.dead[path] = true
	return fmt.Errorf("%s: ни одно имя модели не подошло: %w", path, last)
}

func asStatus(err error, target **statusError) bool {
	se, ok := err.(*statusError)
	if ok {
		*target = se
	}
	return ok
}

func New(baseURL, apiKey, model string, temperature float64, maxTokens int, timeout time.Duration) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"), APIKey: apiKey, Model: model,
		Temperature: temperature, MaxTokens: maxTokens, HTTP: &http.Client{Timeout: timeout},
	}
}

func (c *Client) post(ctx context.Context, path, key string, payload any, out any) error {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("запрос %s: %w", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return &statusError{code: resp.StatusCode, msg: fmt.Sprintf("%s вернул %d: %s", path, resp.StatusCode, truncate(string(raw), 300))}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("разбор ответа %s: %w", path, err)
	}
	return nil
}

// Chat — генерация с тремя попытками.
func (c *Client) Chat(ctx context.Context, messages []Message) (string, error) {
	payload := map[string]any{"model": c.Model, "messages": messages, "temperature": c.Temperature}
	if c.MaxTokens > 0 {
		payload["max_tokens"] = c.MaxTokens
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		var out struct {
			Choices []struct {
				Message Message `json:"message"`
			} `json:"choices"`
		}
		err := c.post(ctx, "/chat/completions", c.APIKey, payload, &out)
		if err == nil && len(out.Choices) > 0 && strings.TrimSpace(out.Choices[0].Message.Content) != "" {
			return strings.TrimSpace(out.Choices[0].Message.Content), nil
		}
		if err == nil {
			err = fmt.Errorf("модель вернула пустой ответ")
		}
		last = err
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 1500 * time.Millisecond):
		}
	}
	return "", fmt.Errorf("LLM недоступна: %w", last)
}

// Embed — нормированные эмбеддинги (bge-m3).
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	var out struct {
		Data []struct {
			Index     *int      `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	names := candidates(c.EmbedModel, "baai/bge-m3", "BAAI/bge-m3", "bge-m3")
	err := c.postModel(ctx, "/embeddings", c.EmbedKey, names, map[string]any{"input": texts, "encoding_format": "float"}, &out)
	if err != nil {
		return nil, err
	}
	vecs := make([][]float64, len(texts))
	for pos, d := range out.Data {
		i := pos
		if d.Index != nil {
			i = *d.Index
		}
		if i >= 0 && i < len(vecs) {
			vecs[i] = kb.Normalize(d.Embedding)
		}
	}
	for _, v := range vecs {
		if v == nil {
			return nil, fmt.Errorf("эмбеддер вернул не все векторы")
		}
	}
	return vecs, nil
}

// Rerank — индексы документов по убыванию релевантности.
func (c *Client) Rerank(ctx context.Context, query string, docs []string, topN int) ([]int, error) {
	var out struct {
		Results []struct {
			Index int `json:"index"`
		} `json:"results"`
		Data []struct {
			Index int `json:"index"`
		} `json:"data"`
	}
	names := candidates(c.RerankModel, "qwen/qwen3-reranker-8b", "Qwen/Qwen3-Reranker-8B", "qwen3-reranker-8b", "Qwen3-Reranker-8B")
	err := c.postModel(ctx, "/rerank", c.RerankKey, names, map[string]any{"query": query, "documents": docs, "top_n": min(topN, len(docs))}, &out)
	if err != nil {
		return nil, err
	}
	items := out.Results
	if len(items) == 0 {
		items = out.Data
	}
	idx := make([]int, 0, len(items))
	for _, it := range items {
		idx = append(idx, it.Index)
	}
	return idx, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

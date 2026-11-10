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
		return fmt.Errorf("%s вернул %d: %s", path, resp.StatusCode, truncate(string(raw), 300))
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
	err := c.post(ctx, "/embeddings", c.EmbedKey, map[string]any{"model": c.EmbedModel, "input": texts, "encoding_format": "float"}, &out)
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
	err := c.post(ctx, "/rerank", c.RerankKey, map[string]any{"model": c.RerankModel, "query": query, "documents": docs, "top_n": min(topN, len(docs))}, &out)
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

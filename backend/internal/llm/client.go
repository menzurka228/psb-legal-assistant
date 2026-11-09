// Package llm — клиент OpenAI-совместимого Chat Completions API (DeepSeek и др.).
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
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Chatter — контракт для генерации ответа (позволяет подменять модель в тестах).
type Chatter interface {
	Chat(ctx context.Context, messages []Message) (string, error)
}

type Client struct {
	BaseURL     string
	APIKey      string
	Model       string
	Temperature float64
	HTTP        *http.Client
}

func New(baseURL, apiKey, model string, temperature float64, timeout time.Duration) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"), APIKey: apiKey, Model: model,
		Temperature: temperature, HTTP: &http.Client{Timeout: timeout},
	}
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

func (c *Client) Chat(ctx context.Context, messages []Message) (string, error) {
	body, _ := json.Marshal(chatRequest{Model: c.Model, Messages: messages, Temperature: c.Temperature})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("запрос к LLM: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM вернула %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("разбор ответа LLM: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("LLM вернула пустой ответ")
	}
	return out.Choices[0].Message.Content, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Сервер принимает только одно имя реранкера — клиент должен его подобрать и запомнить.
func TestRerankPicksWorkingModelName(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		seen = append(seen, body["model"].(string))
		if body["model"] != "Qwen/Qwen3-Reranker-8B" {
			http.Error(w, `{"error":"model not found"}`, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"index":1},{"index":0}]}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "", "m", 0, 0, 5*time.Second)
	c.RerankModel = "qwen/qwen3-reranker-8b"
	for i := 0; i < 2; i++ {
		idx, err := c.Rerank(context.Background(), "q", []string{"a", "b"}, 2)
		if err != nil || len(idx) != 2 || idx[0] != 1 {
			t.Fatalf("rerank: %v %v", idx, err)
		}
	}
	if len(seen) != 3 || seen[2] != "Qwen/Qwen3-Reranker-8B" {
		t.Fatalf("ожидали 2 попытки подбора + 1 запрос с запомненным именем, было: %v", seen)
	}
}

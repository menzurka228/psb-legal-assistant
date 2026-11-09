// Сервер юридического ассистента.
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/menzurka228/psb-legal-assistant/backend/internal/api"
	"github.com/menzurka228/psb-legal-assistant/backend/internal/assistant"
	"github.com/menzurka228/psb-legal-assistant/backend/internal/config"
	"github.com/menzurka228/psb-legal-assistant/backend/internal/kb"
	"github.com/menzurka228/psb-legal-assistant/backend/internal/llm"
)

func main() {
	cfg := config.Load()

	base, err := kb.Load(cfg.KBDir)
	if err != nil {
		log.Fatalf("загрузка базы знаний: %v", err)
	}
	log.Printf("база знаний: %d документов, %d фрагментов (%s)", len(base.Documents), len(base.Chunks), cfg.KBDir)
	if cfg.LLMAPIKey == "" {
		log.Printf("ВНИМАНИЕ: LLM_API_KEY не задан — запросы к модели могут не пройти")
	}

	srv := &api.Server{
		KB: base,
		Assistant: &assistant.Assistant{
			Search:   base,
			LLM:      llm.New(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMTemperature, cfg.LLMTimeout),
			TopK:     cfg.TopK,
			MinScore: cfg.MinScore,
		},
		StaticDir: cfg.StaticDir,
	}

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("слушаю %s", cfg.Addr)
	log.Fatal(httpSrv.ListenAndServe())
}

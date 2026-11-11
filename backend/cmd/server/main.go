// Сервер юридического ассистента: API + веб-интерфейс.
package main

import (
	"context"
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
	log.Printf("база знаний (%s): документов компании %d (разделов %d), законов %d (фрагментов %d)",
		cfg.KBDir, len(base.Company), len(base.CompanyChunks), len(base.Laws), len(base.LawChunks))
	var answerer api.Answerer
	if cfg.DemoMode {
		log.Print("деморежим: локальный поиск по базе")
		answerer = assistant.NewDemo(base)
	} else {
		if cfg.LLMBaseURL == "" {
			log.Printf("ВНИМАНИЕ: LLM_BASE_URL не задан — ответы модели будут недоступны")
		}

		client := llm.New(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMTemperature, cfg.LLMMaxTokens, cfg.LLMTimeout)
		client.EmbedKey, client.EmbedModel = cfg.EmbedKey, cfg.EmbedModel
		client.RerankKey, client.RerankModel = cfg.RerankKey, cfg.RerankModel
		var emb kb.Embedder
		if cfg.UseEmbeddings && cfg.LLMBaseURL != "" {
			emb = client
		}

		a := assistant.New(base, client, emb, cfg.Today, cfg.CompanyFullLimit)
		a.LawCandidates, a.LawTopN, a.SelfCheck = cfg.LawCandidates, cfg.LawTopN, cfg.SelfCheck
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		a.Prepare(ctx)
		cancel()
		answerer = a
	}

	srv := &api.Server{KB: base, Assistant: answerer, StaticDir: cfg.StaticDir}
	httpSrv := &http.Server{Addr: cfg.Addr, Handler: srv.Routes(), ReadHeaderTimeout: 10 * time.Second}
	log.Printf("слушаю %s", cfg.Addr)
	log.Fatal(httpSrv.ListenAndServe())
}

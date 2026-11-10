// Package config читает настройки сервиса из переменных окружения (имена совпадают с платформой хакатона).
package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr      string
	KBDir     string
	StaticDir string

	LLMBaseURL     string
	LLMAPIKey      string
	LLMModel       string
	LLMTemperature float64
	LLMMaxTokens   int
	LLMTimeout     time.Duration

	EmbedKey      string
	EmbedModel    string
	RerankKey     string
	RerankModel   string
	UseEmbeddings bool // эмбеддинги + реранкер; при сбое API — автоматически только BM25

	LawCandidates    int  // фрагментов законов в реранкер
	LawTopN          int  // фрагментов законов в промпт
	CompanyFullLimit int  // если документы компании длиннее — тоже ищем по фрагментам
	SelfCheck        bool // второй проход модели: проверка ответа по базе
	Today            time.Time
}

func Load() Config {
	key := first(os.Getenv("LLM_API_KEY"), os.Getenv("OPENAI_API_KEY"))
	return Config{
		Addr:           env("ADDR", ":8000"),
		KBDir:          env("KB_DIR", "data/kb"),
		StaticDir:      env("STATIC_DIR", "frontend/dist"),
		LLMBaseURL:     first(os.Getenv("LLM_BASE_URL"), os.Getenv("BASE_URL"), os.Getenv("OPENAI_BASE_URL")),
		LLMAPIKey:      key,
		LLMModel:       env("LLM_MODEL", "deepseek/deepseek-v4.1-flash"),
		LLMTemperature: envFloat("LLM_TEMPERATURE", 0.0),
		LLMMaxTokens:   int(envFloat("MAX_TOKENS", 1500)),
		LLMTimeout:     time.Duration(envFloat("LLM_TIMEOUT_SEC", 120)) * time.Second,

		EmbedKey:      first(os.Getenv("EMBEDDER_API_KEY"), key),
		EmbedModel:    env("EMB_MODEL", "baai/bge-m3"),
		RerankKey:     first(os.Getenv("RERANKER_API_KEY"), key),
		RerankModel:   env("RERANKER_MODEL", "qwen/qwen3-reranker-8b"),
		UseEmbeddings: env("USE_EMBEDDINGS", "1") == "1",

		LawCandidates:    int(envFloat("LAW_CANDIDATES", 30)),
		LawTopN:          int(envFloat("LAW_TOP_N", 8)),
		CompanyFullLimit: int(envFloat("COMPANY_FULL_LIMIT", 60000)),
		SelfCheck:        env("SELF_CHECK", "0") == "1",
		Today:            envDate("TODAY"),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func envFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil {
		return v
	}
	return def
}

func envDate(key string) time.Time {
	if t, err := time.Parse("02.01.2006", os.Getenv(key)); err == nil {
		return t
	}
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

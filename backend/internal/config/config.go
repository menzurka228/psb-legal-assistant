// Package config читает настройки сервиса из переменных окружения.
package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr      string // адрес HTTP-сервера
	KBDir     string // папка базы знаний
	StaticDir string // собранный фронтенд

	LLMBaseURL     string
	LLMAPIKey      string
	LLMModel       string
	LLMTemperature float64
	LLMTimeout     time.Duration

	TopK     int     // сколько фрагментов передавать модели
	MinScore float64 // порог BM25 лучшего фрагмента, ниже — отказ без вызова LLM
}

func Load() Config {
	return Config{
		Addr:           env("ADDR", ":8000"),
		KBDir:          env("KB_DIR", "data/kb"),
		StaticDir:      env("STATIC_DIR", "frontend/dist"),
		LLMBaseURL:     env("LLM_BASE_URL", "https://api.deepseek.com/v1"),
		LLMAPIKey:      env("LLM_API_KEY", ""),
		LLMModel:       env("LLM_MODEL", "deepseek-v4.1-flash"),
		LLMTemperature: envFloat("LLM_TEMPERATURE", 0.1),
		LLMTimeout:     time.Duration(envFloat("LLM_TIMEOUT_SEC", 60)) * time.Second,
		TopK:           int(envFloat("TOP_K", 8)),
		MinScore:       envFloat("MIN_SCORE", 1.0),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil {
		return v
	}
	return def
}

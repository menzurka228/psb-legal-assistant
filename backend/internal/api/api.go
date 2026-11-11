// Package api — HTTP-слой: JSON API и раздача собранного фронтенда.
package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/menzurka228/psb-legal-assistant/backend/internal/assistant"
	"github.com/menzurka228/psb-legal-assistant/backend/internal/kb"
	"github.com/menzurka228/psb-legal-assistant/backend/internal/llm"
)

type Answerer interface {
	Answer(context.Context, string, []llm.Message) (assistant.Answer, error)
}

type Server struct {
	KB        *kb.KnowledgeBase
	Assistant Answerer
	StaticDir string
}

type chatRequest struct {
	Question string        `json:"question"`
	History  []llm.Message `json:"history"`
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("POST /api/chat", s.chat)
	mux.HandleFunc("GET /api/documents", s.listDocuments)
	mux.HandleFunc("GET /api/documents/{name...}", s.getDocument)
	mux.Handle("/", s.spa())
	return logRequests(mux)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok", "documents": len(s.KB.Docs), "company": len(s.KB.Company), "laws": len(s.KB.Laws),
		"chunks": len(s.KB.CompanyChunks) + len(s.KB.LawChunks),
	})
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный JSON")
		return
	}
	req.Question = strings.TrimSpace(req.Question)
	if req.Question == "" {
		writeError(w, http.StatusBadRequest, "пустой вопрос")
		return
	}
	ans, err := s.Assistant.Answer(r.Context(), req.Question, req.History)
	if err != nil {
		log.Printf("chat: %v", err)
		writeError(w, http.StatusBadGateway, "не удалось получить ответ модели: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ans)
}

func (s *Server) listDocuments(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.KB.Docs) // name, kind, title
}

func (s *Server) getDocument(w http.ResponseWriter, r *http.Request) {
	d, ok := s.KB.ByName[r.PathValue("name")]
	if !ok {
		writeError(w, http.StatusNotFound, "документ не найден")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": d.Name, "title": d.Title, "kind": d.Kind, "text": d.Text})
}

// spa раздаёт статику фронтенда, а неизвестные пути отдаёт index.html.
func (s *Server) spa() http.Handler {
	files := http.FileServer(http.Dir(s.StaticDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(s.StaticDir, filepath.Clean("/"+r.URL.Path))
		if st, err := os.Stat(path); err != nil || st.IsDir() {
			index := filepath.Join(s.StaticDir, "index.html")
			if _, err := os.Stat(index); err != nil {
				http.Error(w, "Фронтенд не собран: выполните npm run build в frontend/", http.StatusNotFound)
				return
			}
			http.ServeFile(w, r, index)
			return
		}
		files.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s", r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}

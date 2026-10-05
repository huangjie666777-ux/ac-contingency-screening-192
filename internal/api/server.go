package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/huangjie666777-ux/ac-contingency-screening-192/internal/pf"
)

const maxRequestBodyBytes = 1 << 20

// Server wires the power-flow screening engine to HTTP routes.
type Server struct {
	router http.Handler
}

// NewServer constructs the Chi router and middleware stack.
func NewServer() *Server {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	s := &Server{}
	r.Get("/healthz", s.health)
	r.Post("/api/v1/screen", s.screen)
	s.router = r
	return s
}

func (s *Server) Handler() http.Handler { return s.router }

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type errorResponse struct {
	Error string `json:"error"`
}

func (s *Server) screen(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "request body too large or unreadable"})
		return
	}

	var in pf.Case
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: " + err.Error()})
		return
	}
	if err := in.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	report := pf.Screen(&in)
	writeJSON(w, http.StatusOK, report)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

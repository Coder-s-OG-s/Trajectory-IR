package console

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	envAuthToken = "TRAJIR_CONSOLE_TOKEN"
	maxBodyBytes = 1 << 20 // 1 MiB
)

// Server serves local console ingest/read HTTP endpoints.
type Server struct {
	Store *Store
	Token string // empty disables auth (local demos only)
	Mux   *http.ServeMux
}

// NewServer builds routes. Token defaults from TRAJIR_CONSOLE_TOKEN.
func NewServer(store *Store, token string) *Server {
	if token == "" {
		token = strings.TrimSpace(os.Getenv(envAuthToken))
	}
	s := &Server{Store: store, Token: token, Mux: http.NewServeMux()}
	s.Mux.HandleFunc("POST /v1/events", s.handlePostEvent)
	s.Mux.HandleFunc("GET /v1/trajectories", s.handleList)
	s.Mux.HandleFunc("GET /v1/trajectories/{id}/events", s.handleEvents)
	s.Mux.HandleFunc("GET /v1/trajectories/{id}/summary", s.handleSummary)
	s.Mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	s.mountUI()
	return s
}

func (s *Server) mountUI() {
	sub, err := fs.Sub(UI, "web")
	if err != nil {
		// Package is broken if embed fails; surface via a clear handler.
		s.Mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
			writeErr(w, http.StatusInternalServerError, "ui assets unavailable")
		})
		return
	}
	fileServer := http.FileServer(http.FS(sub))
	s.Mux.Handle("GET /ui/", http.StripPrefix("/ui/", fileServer))
	s.Mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, sub, "index.html")
	})
	s.Mux.HandleFunc("GET /ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	})
}

func (s *Server) Handler() http.Handler { return s.Mux }

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) bool {
	if s.Token == "" {
		return true
	}
	got := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(got, prefix) || !subtleConstantTimeEq(got[len(prefix):], s.Token) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	return true
}

func subtleConstantTimeEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func (s *Server) handlePostEvent(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read body")
		return
	}
	if len(body) > maxBodyBytes {
		writeErr(w, http.StatusRequestEntityTooLarge, "body too large")
		return
	}
	e, err := ParseEvent(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Store.Append(e); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":        "accepted",
		"id":            e.ID,
		"trajectory_id": e.TrajectoryID,
	})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	ids, err := s.Store.ListTrajectories()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"trajectories": ids})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	id := r.PathValue("id")
	events, err := s.Store.ReadEvents(id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidEvent) {
			status = http.StatusNotFound
			if errors.Is(err, ErrInvalidEvent) {
				status = http.StatusBadRequest
			}
		}
		writeErr(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"trajectory_id": id,
		"events":        events,
	})
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	id := r.PathValue("id")
	events, err := s.Store.ReadEvents(id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrNotFound) {
			status = http.StatusNotFound
		}
		writeErr(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, Summarize(id, events))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ListenAndServe is a convenience wrapper with timeouts.
func ListenAndServe(addr string, h http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return srv.ListenAndServe()
}

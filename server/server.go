package server

import (
	"errors"
	"io"
	"net/http"

	"stratadb/db"
)

// Server wraps a DB and exposes it over HTTP.
//
// Routes:
//
//	GET    /keys/{key}   → 200 value | 404
//	PUT    /keys/{key}   → 200 (body is the value to store)
//	DELETE /keys/{key}   → 200
//
// Each request runs in its own goroutine (net/http does this automatically).
// Concurrent GETs run in parallel via db's RWMutex; PUTs and DELETEs serialize.
type Server struct {
	db  *db.DB
	mux *http.ServeMux
}

// New creates a Server backed by the given DB and registers all routes.
func New(database *db.DB) *Server {
	s := &Server{db: database, mux: http.NewServeMux()}
	s.mux.HandleFunc("/keys/", s.handleKeys)
	return s
}

// Handler returns the http.Handler so the caller can pass it to http.ListenAndServe.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// handleKeys dispatches GET / PUT / DELETE on /keys/{key}.
func (s *Server) handleKeys(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Path[len("/keys/"):]
	if key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGet(w, r, key)
	case http.MethodPut:
		s.handlePut(w, r, key)
	case http.MethodDelete:
		s.handleDelete(w, r, key)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGet retrieves a key and writes the value to the response body.
// Returns 404 if the key does not exist.
func (s *Server) handleGet(w http.ResponseWriter, _ *http.Request, key string) {
	val, err := s.db.Get(key)
	if errors.Is(err, db.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte(val))
}

// handlePut reads the request body as the value and stores key→value.
func (s *Server) handlePut(w http.ResponseWriter, r *http.Request, key string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.db.Put(key, string(body)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleDelete removes a key.
func (s *Server) handleDelete(w http.ResponseWriter, _ *http.Request, key string) {
	if err := s.db.Delete(key); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// Package health provides HTTP health and readiness check endpoints.
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// CheckFunc is a function that checks a dependency's health.
type CheckFunc func(ctx context.Context) error

// Checker manages health check registrations and evaluations.
type Checker struct {
	mu     sync.RWMutex
	checks map[string]CheckFunc
}

// NewChecker creates a new health checker.
func NewChecker() *Checker {
	return &Checker{
		checks: make(map[string]CheckFunc),
	}
}

// Register adds a named health check.
func (c *Checker) Register(name string, check CheckFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks[name] = check
}

// CheckAll runs all registered checks and returns results.
func (c *Checker) CheckAll(ctx context.Context) map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	results := make(map[string]string, len(c.checks))
	for name, check := range c.checks {
		checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		if err := check(checkCtx); err != nil {
			results[name] = fmt.Sprintf("unhealthy: %s", err.Error())
		} else {
			results[name] = "healthy"
		}
		cancel()
	}
	return results
}

// IsReady returns true if all checks pass.
func (c *Checker) IsReady(ctx context.Context) bool {
	results := c.CheckAll(ctx)
	for _, status := range results {
		if status != "healthy" {
			return false
		}
	}
	return true
}

// Server provides HTTP endpoints for health checks.
type Server struct {
	server  *http.Server
	checker *Checker
}

// NewServer creates a new health server.
func NewServer(port int, checker *Checker) *Server {
	mux := http.NewServeMux()
	s := &Server{
		checker: checker,
		server: &http.Server{
			Addr:         fmt.Sprintf(":%d", port),
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
			IdleTimeout:  30 * time.Second,
		},
	}

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)

	return s
}

// Start begins serving health endpoints (non-blocking).
func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the health server.
func (s *Server) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	results := s.checker.CheckAll(ctx)

	w.Header().Set("Content-Type", "application/json")

	allHealthy := true
	for _, status := range results {
		if status != "healthy" {
			allHealthy = false
			break
		}
	}

	if allHealthy {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": func() string {
			if allHealthy {
				return "ready"
			}
			return "not_ready"
		}(),
		"checks": results,
	})
}

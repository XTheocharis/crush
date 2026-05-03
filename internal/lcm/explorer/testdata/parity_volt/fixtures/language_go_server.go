// Go fixture with symbols and imports for parity testing
package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"gorm.io/gorm"
)

// Server represents an HTTP server with database support.
type Server struct {
	Router      *mux.Router
	DB          *gorm.DB
	Port        int
	Timeout     time.Duration
	middleware  []Middleware
}

// Middleware represents a middleware function.
type Middleware func(http.Handler) http.Handler

// NewServer creates a new server instance.
func NewServer(db *gorm.DB, port int) *Server {
	return &Server{
		Router:  mux.NewRouter(),
		DB:      db,
		Port:    port,
		Timeout: 30 * time.Second,
	}
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.Port)
	return http.ListenAndServe(addr, s.Router)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}

// AddMiddleware adds middleware to the server.
func (s *Server) AddMiddleware(m Middleware) {
	s.middleware = append(s.middleware, m)
}

// routeHandler handles HTTP routes.
type routeHandler struct {
	routes []Route
}

type Route struct {
	Path    string
	Method  string
	Handler http.HandlerFunc
}

func (rh *routeHandler) handle(w http.ResponseWriter, r *http.Request) {
	for _, route := range rh.routes {
		if strings.EqualFold(r.URL.Path, route.Path) &&
			strings.EqualFold(r.Method, route.Method) {
			route.Handler(w, r)
			return
		}
	}
	http.NotFound(w, r)
}
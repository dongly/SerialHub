// Package transport provides MCP transport implementations.
package transport

import (
	"context"
	"net/http"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
)

// HTTPHandler wraps MCP HTTP+SSE handler
type HTTPHandler struct {
	mcpServer  *mcpsdk.Server
	sseHandler *mcpsdk.SSEHandler
	httpServer *http.Server
	mux        *http.ServeMux
	logger     *logrus.Logger
}

// NewHTTPHandler creates a new HTTP+SSE handler for MCP communication
func NewHTTPHandler(mcpServer *mcpsdk.Server, addr string, logger *logrus.Logger) (*HTTPHandler, *http.Server, error) {
	if logger == nil {
		logger = logrus.New()
	}

	mux := http.NewServeMux()

	// Create SSE handler
	sseHandler := mcpsdk.NewSSEHandler(func(r *http.Request) *mcpsdk.Server {
		return mcpServer
	}, &mcpsdk.SSEOptions{})

	// Setup routes
	mux.Handle("/mcp", sseHandler)
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/sse", handleSSE)

	handler := &HTTPHandler{
		mcpServer:  mcpServer,
		sseHandler: sseHandler,
		mux:        mux,
		logger:     logger,
	}

	// Create HTTP server with CORS
	server := &http.Server{
		Addr:         addr,
		Handler:      withCORS(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return handler, server, nil
}

// handleHealth handles health check requests
func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{\"status\":\"ok\"}"))
}

// handleSSE handles SSE connections
func handleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Keep connection alive
	<-r.Context().Done()
}

// withCORS adds CORS support to the HTTP handler
func withCORS(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		handler.ServeHTTP(w, r)
	})
}

// Close stops the HTTP server
func (h *HTTPHandler) Close(server *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h.logger.Infoln("[SerialHub] HTTP+SSE 服务器关闭中...")
	return server.Shutdown(ctx)
}

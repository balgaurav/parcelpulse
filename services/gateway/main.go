package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type dependency struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Latency int64  `json:"latencyMs"`
}

type gateway struct {
	trackingURL      *url.URL
	notificationsURL *url.URL
	client           *http.Client
	logger           *slog.Logger
}

func newGateway() *gateway {
	tracking, _ := url.Parse(envOr("TRACKING_URL", "http://localhost:8081"))
	notifications, _ := url.Parse(envOr("NOTIFICATIONS_URL", "http://localhost:8082"))
	return &gateway{
		trackingURL: tracking, notificationsURL: notifications,
		client: &http.Client{Timeout: 900 * time.Millisecond},
		logger: slog.New(slog.NewJSONHandler(os.Stdout, nil)),
	}
}

func (g *gateway) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", g.health)
	mux.Handle("/api/shipments", g.proxy(g.trackingURL, "/v1"))
	mux.Handle("/api/shipments/", g.proxy(g.trackingURL, "/v1"))
	mux.Handle("/api/notifications", g.proxy(g.notificationsURL, "/v1"))
	return g.withCORS(g.withRequestID(mux))
}

func (g *gateway) health(w http.ResponseWriter, _ *http.Request) {
	targets := []struct{ name string; target *url.URL }{
		{"tracking", g.trackingURL},
		{"notifications", g.notificationsURL},
	}
	results := make([]dependency, len(targets))
	var group sync.WaitGroup
	for i, target := range targets {
		group.Add(1)
		go func(i int, target struct{ name string; target *url.URL }) {
			defer group.Done()
			start := time.Now()
			response, err := g.client.Get(target.target.String() + "/health")
			result := dependency{Name: target.name, Status: "unavailable", Latency: time.Since(start).Milliseconds()}
			if err == nil && response.StatusCode == http.StatusOK {
				result.Status = "healthy"
			}
			if response != nil {
				response.Body.Close()
			}
			results[i] = result
		}(i, target)
	}
	group.Wait()

	overall := "healthy"
	for _, result := range results {
		if result.Status != "healthy" {
			overall = "degraded"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "gateway", "status": overall, "checkedAt": time.Now().UTC(), "dependencies": results,
	})
}

func (g *gateway) proxy(target *url.URL, prefix string) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(target)
	original := proxy.Director
	proxy.Director = func(r *http.Request) {
		original(r)
		r.URL.Path = prefix + strings.TrimPrefix(r.URL.Path, "/api")
		r.Host = target.Host
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		g.logger.Error("upstream failed", "path", r.URL.Path, "target", target.String(), "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream service unavailable"})
	}
	return proxy
}

func (g *gateway) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = time.Now().UTC().Format("20060102T150405.000000000")
		}
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r)
	})
}

func (g *gateway) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	gateway := newGateway()
	gateway.logger.Info("gateway started", "address", ":8080")
	if err := http.ListenAndServe(":8080", gateway.routes()); err != nil {
		gateway.logger.Error("gateway stopped", "error", err)
		os.Exit(1)
	}
}

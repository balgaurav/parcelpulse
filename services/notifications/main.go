package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

type eventEnvelope struct {
	Type     string          `json:"type"`
	Shipment json.RawMessage `json:"shipment"`
	Event    struct {
		ID     string    `json:"id"`
		Status string    `json:"status"`
		At     time.Time `json:"at"`
	} `json:"event"`
}

type notification struct {
	ID        string    `json:"id"`
	Channel   string    `json:"channel"`
	Subject   string    `json:"subject"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type auditStore struct {
	mu    sync.RWMutex
	items []notification
}

func (s *auditStore) add(item notification) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, item)
}

func (s *auditStore) list() []notification {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := append([]notification(nil), s.items...)
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items
}

type service struct {
	store  *auditStore
	logger *slog.Logger
}

func (s *service) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "notifications"})
	})
	mux.HandleFunc("GET /v1/notifications", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"notifications": s.store.list()})
	})
	mux.HandleFunc("POST /internal/events", s.consumeEvent)
	return mux
}

func (s *service) consumeEvent(w http.ResponseWriter, r *http.Request) {
	var envelope eventEnvelope
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&envelope); err != nil || envelope.Type != "shipment.status_changed" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expected shipment.status_changed event"})
		return
	}

	subject := "Shipment update"
	if envelope.Event.Status == "out_for_delivery" {
		subject = "Your parcel is out for delivery"
	}
	if envelope.Event.Status == "delivered" {
		subject = "Your parcel was delivered"
	}
	item := notification{
		ID:        "notif_" + envelope.Event.ID,
		Channel:   "email",
		Subject:   subject,
		Status:    "queued",
		CreatedAt: time.Now().UTC(),
	}
	s.store.add(item)
	s.logger.Info("notification queued", "event_id", envelope.Event.ID, "status", envelope.Event.Status)
	writeJSON(w, http.StatusAccepted, item)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	service := &service{store: &auditStore{}, logger: logger}
	logger.Info("notification service started", "address", ":8082")
	if err := http.ListenAndServe(":8082", service.routes()); err != nil {
		logger.Error("notification service stopped", "error", err)
		os.Exit(1)
	}
}

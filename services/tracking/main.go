package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type TrackingEvent struct {
	ID       string    `json:"id"`
	Status   string    `json:"status"`
	Location string    `json:"location"`
	Note     string    `json:"note"`
	At       time.Time `json:"at"`
}

type Shipment struct {
	ID          string          `json:"id"`
	Recipient   string          `json:"recipient"`
	Destination string          `json:"destination"`
	ETA         time.Time       `json:"eta"`
	Events      []TrackingEvent `json:"events"`
}

type store struct {
	mu        sync.RWMutex
	shipments map[string]Shipment
}

func newStore() *store {
	now := time.Now().UTC()
	return &store{shipments: map[string]Shipment{
		"PP-1042": {ID: "PP-1042", Recipient: "Avery Chen", Destination: "Toronto, ON", ETA: now.Add(7 * time.Hour), Events: []TrackingEvent{
			{ID: "evt_001", Status: "label_created", Location: "Mississauga, ON", Note: "Shipping label generated", At: now.Add(-28 * time.Hour)},
			{ID: "evt_002", Status: "in_transit", Location: "Toronto, ON", Note: "Arrived at destination facility", At: now.Add(-3 * time.Hour)},
		}},
		"PP-2088": {ID: "PP-2088", Recipient: "Noah Williams", Destination: "Ottawa, ON", ETA: now.Add(28 * time.Hour), Events: []TrackingEvent{
			{ID: "evt_003", Status: "label_created", Location: "Montreal, QC", Note: "Merchant handed off parcel", At: now.Add(-4 * time.Hour)},
		}},
	}}
}

func (s *store) list() []Shipment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Shipment, 0, len(s.shipments))
	for _, shipment := range s.shipments {
		result = append(result, shipment)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *store) get(id string) (Shipment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	shipment, ok := s.shipments[id]
	return shipment, ok
}

var allowedTransitions = map[string]map[string]bool{
	"label_created":    {"in_transit": true, "exception": true},
	"in_transit":       {"out_for_delivery": true, "exception": true},
	"out_for_delivery": {"delivered": true, "exception": true},
	"exception":        {"in_transit": true},
}

func (s *store) appendEvent(id, status, location, note string) (Shipment, TrackingEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	shipment, ok := s.shipments[id]
	if !ok {
		return Shipment{}, TrackingEvent{}, errors.New("shipment not found")
	}
	if len(shipment.Events) == 0 || !allowedTransitions[shipment.Events[len(shipment.Events)-1].Status][status] {
		return Shipment{}, TrackingEvent{}, fmt.Errorf("cannot transition from %q to %q", shipment.Events[len(shipment.Events)-1].Status, status)
	}
	event := TrackingEvent{ID: fmt.Sprintf("evt_%d", time.Now().UnixNano()), Status: status, Location: location, Note: note, At: time.Now().UTC()}
	shipment.Events = append(shipment.Events, event)
	s.shipments[id] = shipment
	return shipment, event, nil
}

type service struct {
	store           *store
	notificationsURL string
	client          *http.Client
	logger          *slog.Logger
}

func (s *service) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /v1/shipments", s.listShipments)
	mux.HandleFunc("GET /v1/shipments/{id}", s.getShipment)
	mux.HandleFunc("POST /v1/shipments/{id}/events", s.createEvent)
	return s.withCORS(s.withRequestLog(mux))
}

func (s *service) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "tracking"})
}

func (s *service) listShipments(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"shipments": s.store.list()})
}

func (s *service) getShipment(w http.ResponseWriter, r *http.Request) {
	shipment, ok := s.store.get(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "shipment not found"})
		return
	}
	writeJSON(w, http.StatusOK, shipment)
}

func (s *service) createEvent(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Status   string `json:"status"`
		Location string `json:"location"`
		Note     string `json:"note"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON payload"})
		return
	}
	input.Status, input.Location, input.Note = strings.TrimSpace(input.Status), strings.TrimSpace(input.Location), strings.TrimSpace(input.Note)
	if input.Status == "" || input.Location == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "status and location are required"})
		return
	}

	shipment, event, err := s.store.appendEvent(r.PathValue("id"), input.Status, input.Location, input.Note)
	if err != nil {
		status := http.StatusConflict
		if err.Error() == "shipment not found" {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	go s.publish(shipment, event)
	writeJSON(w, http.StatusCreated, event)
}

func (s *service) publish(shipment Shipment, event TrackingEvent) {
	payload := map[string]any{"type": "shipment.status_changed", "shipment": shipment, "event": event}
	body, _ := json.Marshal(payload)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, s.notificationsURL, strings.NewReader(string(body)))
	if err != nil {
		s.logger.Error("build notification request", "error", err)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		s.logger.Warn("notification delivery failed", "error", err)
		return
	}
	response.Body.Close()
}

func (s *service) withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Info("request complete", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(start).Milliseconds())
	})
}

func (s *service) withCORS(next http.Handler) http.Handler {
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

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	notificationsURL := os.Getenv("NOTIFICATIONS_URL")
	if notificationsURL == "" {
		notificationsURL = "http://localhost:8082/internal/events"
	}
	service := &service{store: newStore(), notificationsURL: notificationsURL, client: &http.Client{Timeout: 2 * time.Second}, logger: logger}
	logger.Info("tracking service started", "address", ":8081")
	if err := http.ListenAndServe(":8081", service.routes()); err != nil {
		logger.Error("tracking service stopped", "error", err)
		os.Exit(1)
	}
}

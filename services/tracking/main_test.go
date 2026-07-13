package main

import "testing"

func TestAppendEventAcceptsValidTransition(t *testing.T) {
	store := newStore()
	shipment, event, err := store.appendEvent("PP-1042", "out_for_delivery", "Toronto, ON", "Courier is on route")
	if err != nil {
		t.Fatalf("appendEvent() error = %v", err)
	}
	if event.Status != "out_for_delivery" {
		t.Fatalf("event status = %q, want out_for_delivery", event.Status)
	}
	if got := shipment.Events[len(shipment.Events)-1].ID; got != event.ID {
		t.Fatalf("last event = %q, want %q", got, event.ID)
	}
}

func TestAppendEventRejectsInvalidTransition(t *testing.T) {
	store := newStore()
	_, _, err := store.appendEvent("PP-2088", "delivered", "Ottawa, ON", "Impossible shortcut")
	if err == nil {
		t.Fatal("appendEvent() error = nil, want transition error")
	}
}

func TestAppendEventRejectsUnknownShipment(t *testing.T) {
	store := newStore()
	_, _, err := store.appendEvent("PP-9999", "in_transit", "Toronto, ON", "")
	if err == nil {
		t.Fatal("appendEvent() error = nil, want not found error")
	}
}

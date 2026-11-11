package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type OrderStatus string

const (
	OrderPending   OrderStatus = "PAYMENT_PENDING"
	OrderPaid      OrderStatus = "PAID"
	OrderCancelled OrderStatus = "CANCELLED"
)

type Order struct {
	Ref             string      `json:"ref"`
	AmountMinor     int64       `json:"amount_minor"`
	Currency        string      `json:"currency"`
	Status          OrderStatus `json:"status"`
	ProviderOrderID string      `json:"provider_order_id"`
	CaptureID       string      `json:"capture_id"`
	ClaimToken      string      `json:"claim_token"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

type Store struct {
	mu    sync.Mutex
	byRef map[string]*Order
	path  string
}

func NewStore(path string) *Store {
	s := &Store{byRef: make(map[string]*Order), path: path}
	s.load()
	return s
}

func (s *Store) load() {
	if s.path == "" {
		return
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Error("load merchant orders", "path", s.path, "err", err)
		}
		return
	}
	orders := make(map[string]*Order)
	if err := json.Unmarshal(raw, &orders); err != nil {
		slog.Error("parse merchant orders", "path", s.path, "err", err)
		return
	}
	s.byRef = orders
}

func (s *Store) persistLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create order directory: %w", err)
	}
	raw, err := json.MarshalIndent(s.byRef, "", "  ")
	if err != nil {
		return fmt.Errorf("encode orders: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write orders: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace orders: %w", err)
	}
	return nil
}

func (s *Store) Create(ref string, amountMinor int64, currency string, claimToken string) (*Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	o := &Order{
		Ref:         ref,
		AmountMinor: amountMinor,
		Currency:    currency,
		Status:      OrderPending,
		ClaimToken:  claimToken,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.byRef[ref] = o
	if err := s.persistLocked(); err != nil {
		delete(s.byRef, ref)
		return nil, err
	}
	return o, nil
}

func (s *Store) Get(ref string) (*Order, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.byRef[ref]
	return o, ok
}

func (s *Store) SetProviderOrder(ref, providerOrderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.byRef[ref]
	if !ok {
		return fmt.Errorf("order %s not found", ref)
	}
	o.ProviderOrderID = providerOrderID
	o.UpdatedAt = time.Now().UTC()
	return s.persistLocked()
}

func (s *Store) Snapshot(ref string) (Order, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.byRef[ref]
	if !ok {
		return Order{}, false
	}
	return *o, true
}

func (s *Store) SetCapture(ref, providerOrderID, captureID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.byRef[ref]
	if !ok {
		return fmt.Errorf("order %s not found", ref)
	}
	if providerOrderID != "" {
		o.ProviderOrderID = providerOrderID
	}
	o.CaptureID = captureID
	o.UpdatedAt = time.Now().UTC()
	return s.persistLocked()
}

func (s *Store) MarkPaid(ref, providerOrderID, captureID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.byRef[ref]
	if !ok {
		return fmt.Errorf("order %s not found", ref)
	}
	o.Status = OrderPaid
	if providerOrderID != "" {
		o.ProviderOrderID = providerOrderID
	}
	if captureID != "" {
		o.CaptureID = captureID
	}
	o.UpdatedAt = time.Now().UTC()
	return s.persistLocked()
}

func (s *Store) All() []*Order {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Order, 0, len(s.byRef))
	for _, o := range s.byRef {
		out = append(out, o)
	}
	return out
}

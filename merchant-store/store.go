package main

import (
	"fmt"
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
	Ref           string
	AmountMinor   int64
	Currency      string
	Status        OrderStatus
	ProviderOrderID string
	CaptureID     string
	ClaimToken    string // shown on confirmation page as ownership proof
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Store struct {
	mu   sync.Mutex
	byRef map[string]*Order
}

func NewStore() *Store {
	return &Store{byRef: make(map[string]*Order)}
}

func (s *Store) Create(ref string, amountMinor int64, currency string, claimToken string) *Order {
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
	return o
}

func (s *Store) Get(ref string) (*Order, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.byRef[ref]
	return o, ok
}

func (s *Store) MarkPaid(ref, providerOrderID, captureID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.byRef[ref]
	if !ok {
		return fmt.Errorf("order %s not found", ref)
	}
	o.Status = OrderPaid
	o.ProviderOrderID = providerOrderID
	o.CaptureID = captureID
	o.UpdatedAt = time.Now().UTC()
	return nil
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

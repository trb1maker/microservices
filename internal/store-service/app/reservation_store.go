package app

import (
	"context"
	"sync"
)

type MemoryReservationStore struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func NewMemoryReservationStore() *MemoryReservationStore {
	return &MemoryReservationStore{seen: make(map[string]struct{})}
}

func reservationKey(orderID, operation string) string {
	return orderID + ":" + operation
}

func (s *MemoryReservationStore) Seen(_ context.Context, orderID, operation string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.seen[reservationKey(orderID, operation)]
	return ok, nil
}

func (s *MemoryReservationStore) Mark(_ context.Context, orderID, operation string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[reservationKey(orderID, operation)] = struct{}{}
	return nil
}

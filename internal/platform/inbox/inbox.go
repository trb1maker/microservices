package inbox

import (
	"context"
	"fmt"
	"sync"
)

type Store interface {
	Seen(ctx context.Context, consumer, eventID string) (bool, error)
	Mark(ctx context.Context, consumer, eventID string) error
}

type Binding struct {
	store    Store
	consumer string
}

func ForConsumer(store Store, consumer string) *Binding {
	return &Binding{store: store, consumer: consumer}
}

func (b *Binding) Seen(ctx context.Context, eventID string) (bool, error) {
	seen, err := b.store.Seen(ctx, b.consumer, eventID)
	if err != nil {
		return false, fmt.Errorf("inbox seen: %w", err)
	}
	return seen, nil
}

func (b *Binding) Mark(ctx context.Context, eventID string) error {
	if err := b.store.Mark(ctx, b.consumer, eventID); err != nil {
		return fmt.Errorf("inbox mark: %w", err)
	}
	return nil
}

type MemoryStore struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{seen: make(map[string]struct{})}
}

func (s *MemoryStore) key(consumer, eventID string) string {
	return consumer + ":" + eventID
}

func (s *MemoryStore) Seen(_ context.Context, consumer, eventID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.seen[s.key(consumer, eventID)]
	return ok, nil
}

func (s *MemoryStore) Mark(_ context.Context, consumer, eventID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[s.key(consumer, eventID)] = struct{}{}
	return nil
}

package mongostore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/trb1maker/microservices/internal/platform/inbox"
)

const (
	fieldConsumer = "consumer"
	fieldEventID  = "event_id"
)

type Store struct {
	coll *mongo.Collection
}

func New(db *mongo.Database) *Store {
	return &Store{coll: db.Collection("inbox")}
}

func (s *Store) Seen(ctx context.Context, consumer, eventID string) (bool, error) {
	err := s.coll.FindOne(ctx, bson.M{fieldConsumer: consumer, fieldEventID: eventID}).Err()
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return false, nil
		}
		return false, fmt.Errorf("query mongo inbox: %w", err)
	}
	return true, nil
}

func (s *Store) Mark(ctx context.Context, consumer, eventID string) error {
	_, err := s.coll.UpdateOne(
		ctx,
		bson.M{fieldConsumer: consumer, fieldEventID: eventID},
		bson.M{"$setOnInsert": bson.M{
			fieldConsumer:  consumer,
			fieldEventID:   eventID,
			"processed_at": time.Now().UTC(),
		}},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("insert mongo inbox: %w", err)
	}
	return nil
}

var _ inbox.Store = (*Store)(nil)

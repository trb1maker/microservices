package mongostore

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/trb1maker/microservices/internal/platform/outbox"
)

const (
	fieldID          = "_id"
	fieldNextAttempt = "next_attempt_at"
	fieldPublishedAt = "published_at"
	opSet            = "$set"
)

type Store struct {
	coll *mongo.Collection
}

func New(db *mongo.Database) *Store {
	return &Store{coll: db.Collection("outbox")}
}

type document struct {
	ID          int64      `bson:"_id"`
	AggregateID string     `bson:"aggregate_id"`
	EventType   string     `bson:"event_type"`
	Subject     string     `bson:"subject"`
	Payload     []byte     `bson:"payload"`
	CreatedAt   time.Time  `bson:"created_at"`
	PublishedAt *time.Time `bson:"published_at"`
	Attempts    int        `bson:"attempts"`
	LastError   string     `bson:"last_error"`
}

func (s *Store) ClaimDue(ctx context.Context, limit int, lease time.Duration) ([]outbox.Message, error) {
	now := time.Now().UTC()
	filter := bson.M{
		fieldPublishedAt: nil,
		"$or": []bson.M{
			{fieldNextAttempt: nil},
			{fieldNextAttempt: bson.M{"$lte": now}},
		},
	}
	update := bson.M{
		"$inc": bson.M{"attempts": 1},
		opSet:  bson.M{fieldNextAttempt: now.Add(lease)},
	}
	opts := options.FindOneAndUpdate().
		SetSort(bson.D{{Key: fieldID, Value: 1}}).
		SetReturnDocument(options.After)

	messages := make([]outbox.Message, 0, limit)
	for range limit {
		var doc document
		err := s.coll.FindOneAndUpdate(ctx, filter, update, opts).Decode(&doc)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				break
			}
			return nil, fmt.Errorf("claim mongo outbox: %w", err)
		}
		aggregateID, parseErr := uuid.Parse(doc.AggregateID)
		if parseErr != nil {
			return nil, fmt.Errorf("parse outbox aggregate id: %w", parseErr)
		}
		messages = append(messages, outbox.Message{
			ID:          doc.ID,
			AggregateID: aggregateID,
			EventType:   doc.EventType,
			Subject:     doc.Subject,
			Payload:     doc.Payload,
			CreatedAt:   doc.CreatedAt,
			Attempts:    doc.Attempts,
		})
	}
	return messages, nil
}

func (s *Store) MarkPublished(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.coll.UpdateMany(ctx, bson.M{fieldID: bson.M{"$in": ids}}, bson.M{
		opSet: bson.M{fieldPublishedAt: time.Now().UTC()},
	})
	if err != nil {
		return fmt.Errorf("mark mongo outbox published: %w", err)
	}
	return nil
}

func (s *Store) Reschedule(ctx context.Context, id int64, nextAttempt time.Time, lastError string) error {
	_, err := s.coll.UpdateOne(ctx, bson.M{fieldID: id}, bson.M{
		opSet: bson.M{fieldNextAttempt: nextAttempt, "last_error": lastError},
	})
	if err != nil {
		return fmt.Errorf("reschedule mongo outbox: %w", err)
	}
	return nil
}

var _ outbox.Store = (*Store)(nil)

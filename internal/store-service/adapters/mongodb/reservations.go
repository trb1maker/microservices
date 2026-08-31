package mongodb

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ReservationStore struct {
	coll *mongo.Collection
}

func NewReservationStore(db *mongo.Database) *ReservationStore {
	return &ReservationStore{coll: db.Collection("reservations")}
}

func (s *ReservationStore) Seen(ctx context.Context, orderID, operation string) (bool, error) {
	err := s.coll.FindOne(ctx, bson.M{fieldOrderID: orderID, fieldOperation: operation}).Err()
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return false, nil
		}
		return false, fmt.Errorf("find reservation: %w", err)
	}
	return true, nil
}

func (s *ReservationStore) Mark(ctx context.Context, orderID, operation string) error {
	_, err := s.coll.UpdateOne(
		ctx,
		bson.M{fieldOrderID: orderID, fieldOperation: operation},
		bson.M{opSetOnInsert: bson.M{fieldOrderID: orderID, fieldOperation: operation}},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("mark reservation: %w", err)
	}
	return nil
}

func EnsureStoreIndexes(ctx context.Context, db *mongo.Database) error {
	_, err := db.Collection("reservations").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: fieldOrderID, Value: 1}, {Key: fieldOperation, Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("create reservations index: %w", err)
	}
	_, err = db.Collection("outbox").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "published_at", Value: 1}, {Key: "next_attempt_at", Value: 1}},
	})
	if err != nil {
		return fmt.Errorf("create outbox index: %w", err)
	}
	_, err = db.Collection("inbox").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "consumer", Value: 1}, {Key: "event_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("create inbox index: %w", err)
	}
	return nil
}

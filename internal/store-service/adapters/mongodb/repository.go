package mongodb

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/trb1maker/microservices/internal/store-service/domain"
)

const (
	fieldID        = "_id"
	fieldProductID = "product_id"
	fieldName      = "name"
	fieldSKU       = "sku"
	fieldPrice     = "price"
	fieldAvailable = "available"
	fieldReserved  = "reserved"
)

// ProductRepository implements app.ProductRepository using MongoDB.
type ProductRepository struct {
	coll *mongo.Collection
}

// NewProductRepository creates a new ProductRepository.
func NewProductRepository(db *mongo.Database) *ProductRepository {
	return &ProductRepository{
		coll: db.Collection("products"),
	}
}

// Get retrieves a product by ID.
func (r *ProductRepository) Get(ctx context.Context, id domain.ProductID) (*domain.Product, error) {
	var doc struct {
		ID    string `bson:"_id"`
		Name  string `bson:"name"`
		SKU   string `bson:"sku"`
		Price int64  `bson:"price"`
	}

	err := r.coll.FindOne(ctx, bson.M{fieldID: string(id)}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrProductNotFound
		}
		return nil, fmt.Errorf("find product: %w", err)
	}

	return &domain.Product{
		ID:    domain.ProductID(doc.ID),
		Name:  doc.Name,
		SKU:   doc.SKU,
		Price: doc.Price,
	}, nil
}

// StockRepository implements app.StockRepository using MongoDB.
type StockRepository struct {
	coll *mongo.Collection
}

// NewStockRepository creates a new StockRepository.
func NewStockRepository(db *mongo.Database) *StockRepository {
	return &StockRepository{
		coll: db.Collection("stock"),
	}
}

// Get retrieves stock by product ID.
func (r *StockRepository) Get(ctx context.Context, productID domain.ProductID) (*domain.Stock, error) {
	var doc struct {
		ProductID string `bson:"product_id"`
		Available int    `bson:"available"`
		Reserved  int    `bson:"reserved"`
	}

	err := r.coll.FindOne(ctx, bson.M{fieldProductID: string(productID)}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrStockNotFound
		}
		return nil, fmt.Errorf("find stock: %w", err)
	}

	return &domain.Stock{
		ProductID: domain.ProductID(doc.ProductID),
		Available: doc.Available,
		Reserved:  doc.Reserved,
	}, nil
}

// Update atomically updates the stock using MongoDB's $set operator.
func (r *StockRepository) Update(ctx context.Context, stock *domain.Stock) error {
	filter := bson.M{fieldProductID: string(stock.ProductID)}
	update := bson.M{
		"$set": bson.M{
			fieldAvailable: stock.Available,
			fieldReserved:  stock.Reserved,
		},
	}

	result, err := r.coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("update stock: %w", err)
	}

	if result.MatchedCount == 0 {
		return domain.ErrStockNotFound
	}

	return nil
}

const (
	demoProductGadgetID = "22222222-2222-4222-8222-222222222222"
	demoProductCableID  = "33333333-3333-4333-8333-333333333333"
	demoProductCaseID   = "44444444-4444-4444-8444-444444444444"
)

// SeedProducts inserts initial demo products and stock.
func SeedProducts(ctx context.Context, db *mongo.Database) error {
	products := []any{
		bson.M{fieldID: demoProductGadgetID, fieldName: "Demo Gadget", fieldSKU: "DEM-001", fieldPrice: 2500},
		bson.M{fieldID: demoProductCableID, fieldName: "USB Cable", fieldSKU: "CBL-001", fieldPrice: 1500},
		bson.M{fieldID: demoProductCaseID, fieldName: "Phone Case", fieldSKU: "CAS-001", fieldPrice: 3500},
		bson.M{fieldID: "prod-1", fieldName: "Laptop", fieldSKU: "LPT-001", fieldPrice: 100000},
		bson.M{fieldID: "prod-2", fieldName: "Mouse", fieldSKU: "MOU-001", fieldPrice: 2500},
		bson.M{fieldID: "prod-3", fieldName: "Keyboard", fieldSKU: "KEY-001", fieldPrice: 5000},
		bson.M{fieldID: "prod-4", fieldName: "Monitor", fieldSKU: "MON-001", fieldPrice: 45000},
		bson.M{fieldID: "prod-5", fieldName: "Headphones", fieldSKU: "HDP-001", fieldPrice: 8000},
	}

	productColl := db.Collection("products")
	for _, p := range products {
		doc := p.(bson.M)
		_, err := productColl.UpdateOne(
			ctx,
			bson.M{fieldID: doc[fieldID]},
			bson.M{"$setOnInsert": p},
			options.Update().SetUpsert(true),
		)
		if err != nil {
			return fmt.Errorf("seed product: %w", err)
		}
	}

	stockItems := []any{
		bson.M{fieldProductID: demoProductGadgetID, fieldAvailable: 100, fieldReserved: 0},
		bson.M{fieldProductID: demoProductCableID, fieldAvailable: 100, fieldReserved: 0},
		bson.M{fieldProductID: demoProductCaseID, fieldAvailable: 100, fieldReserved: 0},
		bson.M{fieldProductID: "prod-1", fieldAvailable: 10, fieldReserved: 0},
		bson.M{fieldProductID: "prod-2", fieldAvailable: 100, fieldReserved: 0},
		bson.M{fieldProductID: "prod-3", fieldAvailable: 50, fieldReserved: 0},
		bson.M{fieldProductID: "prod-4", fieldAvailable: 20, fieldReserved: 0},
		bson.M{fieldProductID: "prod-5", fieldAvailable: 30, fieldReserved: 0},
	}

	stockColl := db.Collection("stock")
	for _, s := range stockItems {
		doc := s.(bson.M)
		_, err := stockColl.UpdateOne(
			ctx,
			bson.M{fieldProductID: doc[fieldProductID]},
			bson.M{"$setOnInsert": s},
			options.Update().SetUpsert(true),
		)
		if err != nil {
			return fmt.Errorf("seed stock: %w", err)
		}
	}

	return nil
}

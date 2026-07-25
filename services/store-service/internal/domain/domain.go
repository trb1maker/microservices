package domain

// ProductID is a domain identifier for a product.
type ProductID string

// Product represents a catalog item.
type Product struct {
	ID    ProductID
	Name  string
	SKU   string
	Price int64 // in minor units
}

// Stock represents the current stock level for a product.
type Stock struct {
	ProductID ProductID
	Available int
	Reserved  int
}

// CanReserve checks if the requested quantity can be reserved.
func (s *Stock) CanReserve(quantity int) bool {
	return s.Available >= quantity
}

// Reserve decreases available and increases reserved.
func (s *Stock) Reserve(quantity int) {
	s.Available -= quantity
	s.Reserved += quantity
}

// Confirm decreases reserved (stock is deducted).
func (s *Stock) Confirm(quantity int) {
	s.Reserved -= quantity
}

// Release decreases reserved and increases available.
func (s *Stock) Release(quantity int) {
	s.Reserved -= quantity
	s.Available += quantity
}

package app

type OrderCreated struct {
	OrderID    string `json:"order_id"`
	UserID     string `json:"user_id"`
	TotalPrice int64  `json:"total_price"`
}

type ReserveItems struct {
	UserID    string `json:"user_id"`
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

type ConfirmOrder struct {
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"`
}

type ReleaseReservation struct {
	UserID  string `json:"user_id"`
	OrderID string `json:"order_id,omitempty"`
}

type OrderFinalized struct {
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"`
}

type OrderCancelled struct {
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"`
}

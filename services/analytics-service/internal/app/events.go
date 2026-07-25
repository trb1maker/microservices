package app

type OrderFinalized struct {
	OrderID     string `json:"order_id"`
	UserID      string `json:"user_id"`
	TotalAmount int64  `json:"total_amount"`
	Status      string `json:"status"`
	FinalizedAt string `json:"finalized_at"`
}

type Receipt struct {
	OrderID     string `json:"order_id"`
	UserID      string `json:"user_id"`
	TotalAmount int64  `json:"total_amount"`
	Status      string `json:"status"`
	FinalizedAt string `json:"finalized_at"`
}

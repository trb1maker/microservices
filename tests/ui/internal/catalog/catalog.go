package catalog

type User struct {
	ID       string
	Email    string
	Password string
	Label    string
}

type Product struct {
	ID    string
	Name  string
	Price int64
	SKU   string
}

var DemoUsers = []User{
	{
		ID:       "11111111-1111-4111-8111-111111111111",
		Email:    "demo@example.com",
		Password: "demo123",
		Label:    "Demo User",
	},
	{
		ID:       "22222222-2222-4222-8222-222222222222",
		Email:    "admin@example.com",
		Password: "admin123",
		Label:    "Admin User",
	},
}

var Products = []Product{
	{ID: "22222222-2222-4222-8222-222222222222", Name: "Demo Gadget", Price: 2500, SKU: "DEM-001"},
	{ID: "33333333-3333-4333-8333-333333333333", Name: "USB Cable", Price: 1500, SKU: "CBL-001"},
	{ID: "44444444-4444-4444-8444-444444444444", Name: "Phone Case", Price: 3500, SKU: "CAS-001"},
}

func UserByEmail(email string) (User, bool) {
	for _, u := range DemoUsers {
		if u.Email == email {
			return u, true
		}
	}
	return User{}, false
}

func ProductByID(id string) (Product, bool) {
	for _, p := range Products {
		if p.ID == id {
			return p, true
		}
	}
	return Product{}, false
}

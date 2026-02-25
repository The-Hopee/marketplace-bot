package marketplace

import "context"

type Product struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Price       float64 `json:"price"`
	OldPrice    float64 `json:"old_price,omitempty"`
	Discount    int     `json:"discount,omitempty"`
	Rating      float64 `json:"rating"`
	ReviewCount int     `json:"review_count"`
	URL         string  `json:"url"`
	ImageURL    string  `json:"image_url"`
	Seller      string  `json:"seller"`
	Marketplace string  `json:"marketplace"`
	InStock     bool    `json:"in_stock"`
}

type SearchResult struct {
	Products   []Product `json:"products"`
	TotalCount int       `json:"total_count"`
	Query      string    `json:"query"`
}

type Marketplace interface {
	Search(ctx context.Context, query string, limit int) (*SearchResult, error)
	GetName() string
}

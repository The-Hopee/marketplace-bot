// internal/marketplace/aggregator.go
package marketplace

import (
	"context"
	"log"
	"sort"
	"sync"
)

type Aggregator struct {
	marketplaces []Marketplace
}

func NewAggregator() *Aggregator {
	return &Aggregator{
		marketplaces: []Marketplace{
			NewWildberries(),
		},
	}
}

type AggregatedResult struct {
	Query      string               `json:"query"`
	Results    map[string][]Product `json:"results"`
	TotalCount int                  `json:"total_count"`
	Errors     map[string]string    `json:"errors,omitempty"`
}

func (a *Aggregator) Search(ctx context.Context, query string, limitPerMarketplace int) *AggregatedResult {
	result := &AggregatedResult{
		Query:   query,
		Results: make(map[string][]Product),
		Errors:  make(map[string]string),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, mp := range a.marketplaces {
		wg.Add(1)
		go func(m Marketplace) {
			defer wg.Done()

			mpName := m.GetName()
			log.Printf("[%s] Starting search for: %s", mpName, query)

			searchResult, err := m.Search(ctx, query, limitPerMarketplace)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				log.Printf("[%s] Error: %v", mpName, err)
				result.Errors[mpName] = err.Error()
				return
			}

			log.Printf("[%s] Found %d products", mpName, len(searchResult.Products))
			result.Results[mpName] = searchResult.Products
			result.TotalCount += len(searchResult.Products)
		}(mp)
	}

	wg.Wait()
	return result
}

func (a *Aggregator) SearchCombined(ctx context.Context, query string, limit int) []Product {
	result := a.Search(ctx, query, limit)

	var allProducts []Product
	for _, products := range result.Results {
		allProducts = append(allProducts, products...)
	}

	// Сортируем по цене
	sort.Slice(allProducts, func(i, j int) bool {
		return allProducts[i].Price < allProducts[j].Price
	})

	if len(allProducts) > limit {
		allProducts = allProducts[:limit]
	}

	return allProducts
}

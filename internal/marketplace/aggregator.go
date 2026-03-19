// internal/marketplace/aggregator.go
package marketplace

import (
	"context"
	"log"
	"sort"
	"sync"
)

type Aggregator struct {
	wb    Marketplace
	ozon  Marketplace
	avito Marketplace
}

func NewAggregator(scraperAPIKey string) *Aggregator {
	return &Aggregator{
		wb:    NewWildberries(),
		ozon:  NewOzon(scraperAPIKey),
		avito: NewAvito(scraperAPIKey),
	}
}

type AggregatedResult struct {
	Query      string               `json:"query"`
	Results    map[string][]Product `json:"results"`
	TotalCount int                  `json:"total_count"`
	Errors     map[string]string    `json:"errors,omitempty"`
}

// Теперь мы передаем subscriptionTier, чтобы знать, какие парсеры включать
func (a *Aggregator) Search(ctx context.Context, query string, limitPerMarketplace int, subscriptionTier string) *AggregatedResult {
	result := &AggregatedResult{
		Query:   query,
		Results: make(map[string][]Product),
		Errors:  make(map[string]string),
	}

	// Определяем, какие маркетплейсы опрашивать
	var activeMarketplaces []Marketplace

	// WB и Ozon доступны всем
	activeMarketplaces = append(activeMarketplaces, a.wb, a.ozon)

	// Avito доступно только Premium и Pro (или админам, у которых HasActiveSubscription() = true)
	if subscriptionTier == "premium" || subscriptionTier == "pro" {
		activeMarketplaces = append(activeMarketplaces, a.avito)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, mp := range activeMarketplaces {
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

			if searchResult != nil && len(searchResult.Products) > 0 {
				log.Printf("[%s] Found %d products", mpName, len(searchResult.Products))
				result.Results[mpName] = searchResult.Products
				result.TotalCount += len(searchResult.Products)
			}
		}(mp)
	}

	wg.Wait()
	return result
}

// Эта функция (если ты её используешь) тоже получает subscriptionTier
func (a *Aggregator) SearchCombined(ctx context.Context, query string, limit int, subscriptionTier string) []Product {
	result := a.Search(ctx, query, limit, subscriptionTier)

	var allProducts []Product
	for _, products := range result.Results {
		allProducts = append(allProducts, products...)
	}

	// Сортируем по цене (от меньшей к большей)
	sort.Slice(allProducts, func(i, j int) bool {
		return allProducts[i].Price < allProducts[j].Price
	})

	if len(allProducts) > limit {
		allProducts = allProducts[:limit]
	}

	return allProducts
}

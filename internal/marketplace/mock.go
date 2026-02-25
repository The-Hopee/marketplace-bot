package marketplace

import (
	"context"
	"math/rand"
	"strings"
	"time"
)

type MockMarketplace struct {
	name     string
	products map[string][]Product
}

func NewMockOzon() *MockMarketplace {
	return &MockMarketplace{
		name: "OZON",
		products: map[string][]Product{
			"default": {
				{ID: "ozon-1", Name: "Популярный товар", Price: 2990, OldPrice: 3990, Discount: 25, Rating: 4.7, ReviewCount: 1250, Marketplace: "OZON", InStock: true},
			},
			"лего": {
				{ID: "ozon-lego-1", Name: "LEGO Star Wars Millennium Falcon 75375", Price: 18990, OldPrice: 21990, Discount: 14, Rating: 4.9, ReviewCount: 342, Marketplace: "OZON", InStock: true},
				{ID: "ozon-lego-2", Name: "LEGO Star Wars AT-AT 75313", Price: 54990, OldPrice: 59990, Discount: 8, Rating: 4.8, ReviewCount: 156, Marketplace: "OZON", InStock: true},
				{ID: "ozon-lego-3", Name: "LEGO Star Wars X-Wing Fighter 75355", Price: 22990, OldPrice: 24990, Discount: 8, Rating: 4.9, ReviewCount: 234, Marketplace: "OZON", InStock: true},
				{ID: "ozon-lego-4", Name: "LEGO Star Wars Darth Vader Helmet 75304", Price: 8990, OldPrice: 9990, Discount: 10, Rating: 4.7, ReviewCount: 567, Marketplace: "OZON", InStock: true},
				{ID: "ozon-lego-5", Name: "LEGO Star Wars The Mandalorian N-1 75325", Price: 6990, OldPrice: 7990, Discount: 12, Rating: 4.6, ReviewCount: 189, Marketplace: "OZON", InStock: true},
			},
			"iphone": {
				{ID: "ozon-ip-1", Name: "Apple iPhone 15 Pro 256GB", Price: 124990, OldPrice: 134990, Discount: 7, Rating: 4.9, ReviewCount: 1250, Marketplace: "OZON", InStock: true},
				{ID: "ozon-ip-2", Name: "Apple iPhone 15 128GB", Price: 79990, OldPrice: 84990, Discount: 6, Rating: 4.8, ReviewCount: 2340, Marketplace: "OZON", InStock: true},
				{ID: "ozon-ip-3", Name: "Apple iPhone 14 128GB", Price: 64990, OldPrice: 74990, Discount: 13, Rating: 4.8, ReviewCount: 4560, Marketplace: "OZON", InStock: true},
			},
			"смартфон": {
				{ID: "ozon-sm-1", Name: "Samsung Galaxy S24 Ultra 256GB", Price: 109990, OldPrice: 119990, Discount: 8, Rating: 4.8, ReviewCount: 890, Marketplace: "OZON", InStock: true},
				{ID: "ozon-sm-2", Name: "Xiaomi 14 Pro 256GB", Price: 74990, OldPrice: 84990, Discount: 12, Rating: 4.7, ReviewCount: 456, Marketplace: "OZON", InStock: true},
				{ID: "ozon-sm-3", Name: "Google Pixel 8 Pro 128GB", Price: 89990, OldPrice: 94990, Discount: 5, Rating: 4.6, ReviewCount: 234, Marketplace: "OZON", InStock: true},
			},
			"наушники": {
				{ID: "ozon-hp-1", Name: "Apple AirPods Pro 2", Price: 22990, OldPrice: 24990, Discount: 8, Rating: 4.9, ReviewCount: 3450, Marketplace: "OZON", InStock: true},
				{ID: "ozon-hp-2", Name: "Sony WH-1000XM5", Price: 29990, OldPrice: 34990, Discount: 14, Rating: 4.8, ReviewCount: 1230, Marketplace: "OZON", InStock: true},
				{ID: "ozon-hp-3", Name: "Samsung Galaxy Buds2 Pro", Price: 12990, OldPrice: 14990, Discount: 13, Rating: 4.7, ReviewCount: 890, Marketplace: "OZON", InStock: true},
			},
		},
	}
}

func NewMockWildberries() *MockMarketplace {
	return &MockMarketplace{
		name: "Wildberries",
		products: map[string][]Product{
			"default": {
				{ID: "wb-1", Name: "Популярный товар", Price: 2890, OldPrice: 3890, Discount: 26, Rating: 4.6, ReviewCount: 2340, Marketplace: "Wildberries", InStock: true},
			},
			"лего": {
				{ID: "wb-lego-1", Name: "LEGO Star Wars Сокол Тысячелетия 75375", Price: 17990, OldPrice: 22990, Discount: 22, Rating: 4.9, ReviewCount: 567, Marketplace: "Wildberries", InStock: true},
				{ID: "wb-lego-2", Name: "LEGO Star Wars AT-AT Шагоход 75313", Price: 52990, OldPrice: 62990, Discount: 16, Rating: 4.8, ReviewCount: 234, Marketplace: "Wildberries", InStock: true},
				{ID: "wb-lego-3", Name: "LEGO Star Wars Истребитель X-Wing 75355", Price: 21490, OldPrice: 25990, Discount: 17, Rating: 4.9, ReviewCount: 345, Marketplace: "Wildberries", InStock: true},
				{ID: "wb-lego-4", Name: "LEGO Star Wars Шлем Дарта Вейдера 75304", Price: 7990, OldPrice: 9990, Discount: 20, Rating: 4.8, ReviewCount: 789, Marketplace: "Wildberries", InStock: true},
				{ID: "wb-lego-5", Name: "LEGO Star Wars Мандалорец N-1 75325", Price: 5990, OldPrice: 7990, Discount: 25, Rating: 4.7, ReviewCount: 234, Marketplace: "Wildberries", InStock: true},
			},
			"iphone": {
				{ID: "wb-ip-1", Name: "Apple iPhone 15 Pro 256GB Черный титан", Price: 121990, OldPrice: 139990, Discount: 13, Rating: 4.9, ReviewCount: 2340, Marketplace: "Wildberries", InStock: true},
				{ID: "wb-ip-2", Name: "Apple iPhone 15 128GB Синий", Price: 77990, OldPrice: 86990, Discount: 10, Rating: 4.8, ReviewCount: 3450, Marketplace: "Wildberries", InStock: true},
				{ID: "wb-ip-3", Name: "Apple iPhone 14 128GB Черный", Price: 62990, OldPrice: 76990, Discount: 18, Rating: 4.8, ReviewCount: 5670, Marketplace: "Wildberries", InStock: true},
			},
			"смартфон": {
				{ID: "wb-sm-1", Name: "Samsung Galaxy S24 Ultra 256GB Черный", Price: 107990, OldPrice: 124990, Discount: 14, Rating: 4.7, ReviewCount: 1560, Marketplace: "Wildberries", InStock: true},
				{ID: "wb-sm-2", Name: "Xiaomi 14 Pro 256GB Белый", Price: 71990, OldPrice: 86990, Discount: 17, Rating: 4.8, ReviewCount: 890, Marketplace: "Wildberries", InStock: true},
				{ID: "wb-sm-3", Name: "Realme GT 5 Pro 256GB", Price: 49990, OldPrice: 59990, Discount: 17, Rating: 4.5, ReviewCount: 345, Marketplace: "Wildberries", InStock: true},
			},
			"наушники": {
				{ID: "wb-hp-1", Name: "Apple AirPods Pro 2 USB-C", Price: 21990, OldPrice: 26990, Discount: 19, Rating: 4.9, ReviewCount: 4560, Marketplace: "Wildberries", InStock: true},
				{ID: "wb-hp-2", Name: "Sony WH-1000XM5 Черные", Price: 27990, OldPrice: 36990, Discount: 24, Rating: 4.9, ReviewCount: 2340, Marketplace: "Wildberries", InStock: true},
				{ID: "wb-hp-3", Name: "JBL Tune 720BT", Price: 4990, OldPrice: 6990, Discount: 29, Rating: 4.5, ReviewCount: 7890, Marketplace: "Wildberries", InStock: true},
			},
		},
	}
}

func (m *MockMarketplace) GetName() string {
	return m.name
}

func (m *MockMarketplace) Search(ctx context.Context, query string, limit int) (*SearchResult, error) {
	// Имитация задержки сети
	time.Sleep(time.Duration(50+rand.Intn(100)) * time.Millisecond)

	queryLower := strings.ToLower(query)

	// Ищем подходящую категорию
	var products []Product

	for category, prods := range m.products {
		if category == "default" {
			continue
		}
		if strings.Contains(queryLower, category) || strings.Contains(category, queryLower) {
			products = prods
			break
		}
	}

	// Если категория не найдена - проверяем ключевые слова
	if len(products) == 0 {
		keywords := map[string]string{
			"star wars":    "лего",
			"звездные":     "лего",
			"звёздные":     "лего",
			"войны":        "лего",
			"конструктор":  "лего",
			"apple":        "iphone",
			"айфон":        "iphone",
			"телефон":      "смартфон",
			"samsung":      "смартфон",
			"xiaomi":       "смартфон",
			"самсунг":      "смартфон",
			"сяоми":        "смартфон",
			"airpods":      "наушники",
			"беспроводные": "наушники",
			"bluetooth":    "наушники",
			"sony":         "наушники",
			"jbl":          "наушники",
		}

		for keyword, category := range keywords {
			if strings.Contains(queryLower, keyword) {
				if prods, ok := m.products[category]; ok {
					products = prods
					break
				}
			}
		}
	}

	// Если всё ещё пусто - используем default
	if len(products) == 0 {
		products = m.products["default"]
	}

	// Ограничиваем количество
	if len(products) > limit {
		products = products[:limit]
	}

	// Копируем и добавляем URL
	result := make([]Product, len(products))
	for i, p := range products {
		result[i] = p
		if m.name == "OZON" {
			result[i].URL = "https://www.ozon.ru/product/" + p.ID
		} else {
			result[i].URL = "https://www.wildberries.ru/catalog/" + p.ID + "/detail.aspx"
		}
	}

	return &SearchResult{
		Products:   result,
		TotalCount: len(result),
		Query:      query,
	}, nil
}

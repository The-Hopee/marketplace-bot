package marketplace

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"marketplace-bot/internal/browser"
)

type OzonMarketplace struct {
	browser *browser.Browser
}

func NewOzon() *OzonMarketplace {
	return &OzonMarketplace{
		browser: browser.GetBrowser(),
	}
}

func (o *OzonMarketplace) GetName() string {
	return "OZON"
}

func (o *OzonMarketplace) Search(ctx context.Context, query string, limit int) (*SearchResult, error) {
	searchURL := fmt.Sprintf("https://www.ozon.ru/search/?text=%s&from_global=true", url.QueryEscape(query))

	log.Printf("[OZON] Loading page: %s", searchURL)

	html, err := o.browser.GetPage(ctx, searchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to load page: %w", err)
	}

	log.Printf("[OZON] Page loaded, size: %d bytes", len(html))

	products := o.parseHTML(html, limit)

	log.Printf("[OZON] Parsed %d products", len(products))

	return &SearchResult{
		Products:   products,
		TotalCount: len(products),
		Query:      query,
	}, nil
}

func (o *OzonMarketplace) parseHTML(html string, limit int) []Product {
	var products []Product

	// OZON хранит данные в JSON внутри страницы
	// Ищем паттерн с данными о товарах
	patterns := []string{
		// Паттерн 1: state в JSON
		`"id":(\d+),"name":"([^"]+)"[^}]*?"price":(\d+)`,
		// Паттерн 2: через sku
		`"sku":(\d+),"title":"([^"]+)"[^}]*?"finalPrice":(\d+)`,
		// Паттерн 3: через cellTrackingInfo
		`"id":"(\d+)"[^}]*?"title":"([^"]+)"[^}]*?"price":"?(\d+)"?`,
		// Паттерн 4: данные в виджетах
		`"cellTrackingInfo":\{[^}]*"id":(\d+)[^}]*\}.*?"title[Ll]ine":"([^"]+)".*?"price":\{[^}]*"price":"?(\d+)"?`,
	}

	seen := make(map[string]bool)

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(html, limit*3)

		for _, match := range matches {
			if len(products) >= limit {
				break
			}
			if len(match) >= 4 {
				id := match[1]

				if seen[id] {
					continue
				}
				seen[id] = true

				name := strings.TrimSpace(match[2])
				name = strings.ReplaceAll(name, "\\u0026", "&")
				name = strings.ReplaceAll(name, "\\\"", "\"")

				priceStr := match[3]
				price, _ := strconv.ParseFloat(priceStr, 64)

				// Иногда цена в копейках
				if price > 1000000 {
					price = price / 100
				}

				if price > 0 && name != "" && len(name) > 3 {
					products = append(products, Product{
						ID:          id,
						Name:        name,
						Price:       price,
						URL:         fmt.Sprintf("https://www.ozon.ru/product/%s", id),
						Marketplace: "OZON",
						InStock:     true,
					})
				}
			}
		}

		if len(products) >= limit {
			break
		}
	}

	// Альтернативный парсинг через HTML теги
	if len(products) == 0 {
		products = o.parseHTMLTags(html, limit)
	}

	return products
}

func (o *OzonMarketplace) parseHTMLTags(html string, limit int) []Product {
	var products []Product

	// Ищем карточки товаров через aria-label и data-атрибуты
	cardPattern := regexp.MustCompile(`href="/product/([^/"]+)[^"]*"[^>]*>.*?aria-label="([^"]+)".*?(\d[\d\s]*)\s*₽`)

	matches := cardPattern.FindAllStringSubmatch(html, limit)

	for _, match := range matches {
		if len(match) >= 4 {
			id := match[1]
			name := strings.TrimSpace(match[2])
			priceStr := match[3]

			price := parsePrice(priceStr)

			if price > 0 && name != "" {
				products = append(products, Product{
					ID:          id,
					Name:        name,
					Price:       price,
					URL:         fmt.Sprintf("https://www.ozon.ru/product/%s", id),
					Marketplace: "OZON",
					InStock:     true,
				})
			}
		}
	}

	return products
}

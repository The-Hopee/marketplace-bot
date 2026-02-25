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

type WildberriesMarketplace struct {
	browser *browser.Browser
}

func NewWildberries() *WildberriesMarketplace {
	return &WildberriesMarketplace{
		browser: browser.GetBrowser(),
	}
}

func (w *WildberriesMarketplace) GetName() string {
	return "Wildberries"
}

func (w *WildberriesMarketplace) Search(ctx context.Context, query string, limit int) (*SearchResult, error) {
	searchURL := fmt.Sprintf("https://www.wildberries.ru/catalog/0/search.aspx?search=%s", url.QueryEscape(query))

	log.Printf("[WB] Loading page: %s", searchURL)

	html, err := w.browser.GetPage(ctx, searchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to load page: %w", err)
	}

	log.Printf("[WB] Page loaded, size: %d bytes", len(html))

	products := w.parseHTML(html, limit)

	log.Printf("[WB] Parsed %d products", len(products))

	return &SearchResult{
		Products:   products,
		TotalCount: len(products),
		Query:      query,
	}, nil
}

func (w *WildberriesMarketplace) parseHTML(html string, limit int) []Product {
	var products []Product

	// Паттерн для JSON данных в HTML
	// WB встраивает данные о товарах в JSON внутри страницы
	jsonPattern := regexp.MustCompile(`"id":(\d+),"root":\d+,"kindId":\d+,"brand":"([^"]*)".*?"name":"([^"]*)".*?"priceU":(\d+).*?"salePriceU":(\d+)`)

	matches := jsonPattern.FindAllStringSubmatch(html, limit*3)

	seen := make(map[string]bool)

	for _, match := range matches {
		if len(products) >= limit {
			break
		}
		if len(match) >= 6 {
			id := match[1]

			// Пропускаем дубликаты
			if seen[id] {
				continue
			}
			seen[id] = true

			brand := match[2]
			name := match[3]
			priceU, _ := strconv.ParseFloat(match[4], 64)
			salePriceU, _ := strconv.ParseFloat(match[5], 64)

			// Цены в копейках
			price := salePriceU / 100
			oldPrice := priceU / 100

			// Вычисляем скидку
			discount := 0
			if oldPrice > 0 && oldPrice > price {
				discount = int((1 - price/oldPrice) * 100)
			}

			fullName := name
			if brand != "" {
				fullName = brand + " / " + name
			}

			products = append(products, Product{
				ID:          id,
				Name:        fullName,
				Price:       price,
				OldPrice:    oldPrice,
				Discount:    discount,
				URL:         fmt.Sprintf("https://www.wildberries.ru/catalog/%s/detail.aspx", id),
				Marketplace: "Wildberries",
				InStock:     true,
			})
		}
	}

	// Если JSON не нашли, пробуем альтернативный паттерн
	if len(products) == 0 {
		products = w.parseHTMLAlternative(html, limit)
	}

	return products
}

func (w *WildberriesMarketplace) parseHTMLAlternative(html string, limit int) []Product {
	var products []Product

	// Альтернативный паттерн
	pattern := regexp.MustCompile(`data-nm-id="(\d+)"[^>]*>.*?<span[^>]*>([^<]+)</span>.*?<ins[^>]*>([^<]+)</ins>`)

	matches := pattern.FindAllStringSubmatch(html, limit)

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
					URL:         fmt.Sprintf("https://www.wildberries.ru/catalog/%s/detail.aspx", id),
					Marketplace: "Wildberries",
					InStock:     true,
				})
			}
		}
	}

	return products
}

func parsePrice(s string) float64 {
	re := regexp.MustCompile(`[\d\s]+`)
	match := re.FindString(s)
	match = strings.ReplaceAll(match, " ", "")
	match = strings.ReplaceAll(match, "\u00a0", "")
	price, _ := strconv.ParseFloat(match, 64)
	return price
}

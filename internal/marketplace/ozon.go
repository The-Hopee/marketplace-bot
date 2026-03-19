// internal/marketplace/ozon.go
package marketplace

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type OzonMarketplace struct {
	ScraperAPIKey string
}

func NewOzon(scraperAPIKey string) *OzonMarketplace {
	return &OzonMarketplace{
		ScraperAPIKey: scraperAPIKey,
	}
}

func (o *OzonMarketplace) GetName() string {
	return "OZON"
}

func (o *OzonMarketplace) Search(ctx context.Context, query string, limit int) (*SearchResult, error) {
	if o.ScraperAPIKey == "" {
		return nil, fmt.Errorf("scraperapi key is missing")
	}

	// Формируем целевой URL
	targetURL := fmt.Sprintf("https://www.ozon.ru/search/?text=%s", url.QueryEscape(query))

	// Оборачиваем в ScraperAPI с флагом render=true (Ozon на React, нужен рендер JS)
	scraperURL := fmt.Sprintf("http://api.scraperapi.com/?api_key=%s&url=%s&render=true",
		o.ScraperAPIKey, url.QueryEscape(targetURL))

	log.Printf("[OZON] Sending request via ScraperAPI: %s", targetURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scraperURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scraperapi request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)
	products := o.parseHTML(html, limit)

	return &SearchResult{
		Products:   products,
		TotalCount: len(products),
		Query:      query,
	}, nil
}

func (o *OzonMarketplace) parseHTML(html string, limit int) []Product {
	var products []Product
	seen := make(map[string]bool)

	// Ищем ссылки на товары. Ozon обычно использует формат /product/название-id/
	pattern := regexp.MustCompile(`href="(/product/[^"]+-([0-9]+)/?)[^"]*"[^>]*>([^<]+)</a>`)
	matches := pattern.FindAllStringSubmatch(html, limit*3)

	for _, match := range matches {
		if len(products) >= limit {
			break
		}

		rawURL := match[1]
		idMatch := match[2]
		name := cleanString(match[3]) // Используем твою функцию из wildberries.go

		// Отсеиваем мусор (отзывы, вопросы)
		if len(name) < 5 || strings.Contains(strings.ToLower(name), "отзыв") || seen[idMatch] {
			continue
		}
		seen[idMatch] = true

		fullURL := "https://www.ozon.ru" + strings.Split(rawURL, "?")[0]

		// Ищем цену где-то поблизости от названия (в радиусе 500 символов)
		// Ozon часто использует символ ₽ или слово "руб"
		price := float64(0)
		idx := strings.Index(html, match[0])
		if idx != -1 {
			endIdx := min(idx+500, len(html))
			nearbyText := html[idx:endIdx]

			pricePattern := regexp.MustCompile(`([0-9\s\x{00A0}]+)₽`)
			priceMatch := pricePattern.FindStringSubmatch(nearbyText)
			if len(priceMatch) > 1 {
				price = extractPrice(priceMatch[1]) // Твоя функция
			}
		}

		products = append(products, Product{
			ID:          idMatch,
			Name:        name,
			Price:       price,
			URL:         fullURL,
			Marketplace: "OZON",
			InStock:     true,
			Condition:   "Новое", // Ozon по умолчанию продает новые товары
		})
	}

	log.Printf("[OZON] Parsed %d products", len(products))
	return products
}

// internal/marketplace/ozon.go
package marketplace

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
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
	targetURL := fmt.Sprintf("https://www.ozon.ru/search/?text=%s", url.QueryEscape(query))
	scraperURL := fmt.Sprintf("http://api.scraperapi.com/?api_key=%s&url=%s&anti_bot=true&country_code=ru", o.ScraperAPIKey, url.QueryEscape(targetURL))

	log.Printf("[OZON] Sending request via ScraperAPI: %s", targetURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scraperURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)

	// СОХРАНЯЕМ HTML ДЛЯ ОТЛАДКИ (как на WB)
	log.Printf("[OZON] Page loaded, size: %d bytes", len(html))
	_ = os.WriteFile("/tmp/debug_ozon.html", body, 0644)

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

	// МАКСИМАЛЬНО АГРЕССИВНЫЙ ПАРСИНГ: ищем любые ссылки на товары
	pattern := regexp.MustCompile(`href="(/product/[^"]+)"`)
	matches := pattern.FindAllStringSubmatch(html, limit*5)

	for _, match := range matches {
		if len(products) >= limit {
			break
		}

		rawURL := match[1]

		// Достаем ID
		idParts := strings.Split(strings.TrimRight(rawURL, "/"), "-")
		idMatch := idParts[len(idParts)-1]

		if seen[idMatch] {
			continue
		}
		seen[idMatch] = true

		fullURL := "https://www.ozon.ru" + strings.Split(rawURL, "?")[0]

		// Ищем цену где-то рядом со ссылкой (в радиусе 1000 символов)
		price := float64(0)
		idx := strings.Index(html, match[0])
		if idx != -1 {
			endIdx := min(idx+1000, len(html))
			nearbyText := html[idx:endIdx]

			pricePattern := regexp.MustCompile(`([0-9\s\x{00A0}]+)₽`)
			if priceMatch := pricePattern.FindStringSubmatch(nearbyText); len(priceMatch) > 1 {
				price = extractPrice(priceMatch[1])
			}
		}

		products = append(products, Product{
			ID:          idMatch,
			Name:        "Товар Ozon " + idMatch, // Пока ставим заглушку, главное достать цену и ссылку
			Price:       price,
			URL:         fullURL,
			Marketplace: "OZON",
			InStock:     true,
			Condition:   "Новое",
		})
	}
	log.Printf("[OZON] Parsed %d products", len(products))
	return products
}

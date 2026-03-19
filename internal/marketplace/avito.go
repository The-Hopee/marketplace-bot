// internal/marketplace/avito.go
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
	"time"
)

type AvitoMarketplace struct {
	ScraperAPIKey string
}

func NewAvito(scraperAPIKey string) *AvitoMarketplace {
	return &AvitoMarketplace{
		ScraperAPIKey: scraperAPIKey,
	}
}

func (a *AvitoMarketplace) GetName() string {
	return "Avito"
}

func (a *AvitoMarketplace) Search(ctx context.Context, query string, limit int) (*SearchResult, error) {
	targetURL := fmt.Sprintf("https://www.avito.ru/all?q=%s", url.QueryEscape(query))
	scraperURL := fmt.Sprintf("http://api.scraperapi.com/?api_key=%s&url=%s&premium=true&country_code=ru", a.ScraperAPIKey, url.QueryEscape(targetURL))

	log.Printf("[AVITO] Sending request: %s", targetURL)

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, scraperURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	// СОХРАНЯЕМ В ПРОБРОШЕННУЮ ПАПКУ
	filename := fmt.Sprintf("debug_html/avito_%d.html", time.Now().Unix())
	_ = os.WriteFile(filename, body, 0644)
	log.Printf("[AVITO] HTML saved to: %s (Size: %d bytes)", filename, len(html))

	products := a.parseHTML(html, limit)
	return &SearchResult{Products: products, TotalCount: len(products), Query: query}, nil
}

func (a *AvitoMarketplace) parseHTML(html string, limit int) []Product {
	var products []Product
	seen := make(map[string]bool)

	// САМАЯ БАЗОВАЯ РЕГУЛЯРКА: ищем тупо все ссылки на объявления
	pattern := regexp.MustCompile(`href="(/[^"]+_([0-9]{8,}))"`)
	matches := pattern.FindAllStringSubmatch(html, limit*5)

	for _, match := range matches {
		if len(products) >= limit {
			break
		}

		rawURL := match[1]
		idMatch := match[2]

		if seen[idMatch] || strings.Contains(rawURL, "/user/") {
			continue
		}
		seen[idMatch] = true

		// Пытаемся найти хоть какую-то цену рядом
		price := float64(0)
		idx := strings.Index(html, match[0])
		if idx != -1 {
			endIdx := min(idx+800, len(html))
			nearbyText := html[idx:endIdx]

			// Ищем просто 5-6 цифр подряд со знаком рубля или словом rub
			pricePattern := regexp.MustCompile(`([0-9\s\x{00A0}]{3,10})(?:₽|rub|руб)`)
			if priceMatch := pricePattern.FindStringSubmatch(strings.ToLower(nearbyText)); len(priceMatch) > 1 {
				price = extractPrice(priceMatch[1])
			}
		}

		products = append(products, Product{
			ID:          idMatch,
			Name:        "Товар Авито " + idMatch,
			Price:       price,
			URL:         "https://www.avito.ru" + rawURL,
			Marketplace: "Avito",
			Condition:   "Б/У",
			InStock:     true,
		})
	}
	log.Printf("[AVITO] Parsed %d products", len(products))
	return products
}

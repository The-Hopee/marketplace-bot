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

	log.Printf("[AVITO] Sending request via ScraperAPI: %s", targetURL)

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

	// СОХРАНЯЕМ HTML ДЛЯ ОТЛАДКИ
	log.Printf("[AVITO] Page loaded, size: %d bytes", len(html))
	_ = os.WriteFile("/tmp/debug_avito.html", body, 0644)

	products := a.parseHTML(html, limit)

	return &SearchResult{
		Products:   products,
		TotalCount: len(products),
		Query:      query,
	}, nil
}

func (a *AvitoMarketplace) parseHTML(html string, limit int) []Product {
	var products []Product
	seen := make(map[string]bool)

	// Ищем ЛЮБЫЕ ссылки, которые заканчиваются на _ и минимум 8 цифр (это ID объявления)
	pattern := regexp.MustCompile(`href="(/[^"]+_[0-9]{8,})"`)
	matches := pattern.FindAllStringSubmatch(html, limit*5)

	for _, match := range matches {
		if len(products) >= limit {
			break
		}

		rawURL := match[1]

		// Достаем ID (всё, что после последнего подчеркивания)
		parts := strings.Split(rawURL, "_")
		idMatch := parts[len(parts)-1]

		// Отсеиваем дубликаты и мусор (например, профили пользователей)
		if seen[idMatch] || strings.Contains(rawURL, "/user/") {
			continue
		}
		seen[idMatch] = true

		fullURL := "https://www.avito.ru" + rawURL

		// Ищем цену рядом со ссылкой
		price := float64(0)
		idx := strings.Index(html, match[0])
		if idx != -1 {
			endIdx := min(idx+800, len(html))
			nearbyText := html[idx:endIdx]

			// Ищем мета-тег цены ИЛИ просто цифры со знаком ₽
			pricePattern := regexp.MustCompile(`(?:content="|">)([0-9\s\x{00A0}]+)(?:&nbsp;)?(?:₽|")`)
			if priceMatch := pricePattern.FindStringSubmatch(nearbyText); len(priceMatch) > 1 {
				price = extractPrice(priceMatch[1])
			}
		}

		products = append(products, Product{
			ID:          idMatch,
			Name:        "Товар Авито " + idMatch, // Название пока заглушка
			Price:       price,
			URL:         fullURL,
			Marketplace: "Avito",
			Condition:   "Б/У",
			InStock:     true,
		})
	}
	log.Printf("[AVITO] Parsed %d products", len(products))
	return products
}

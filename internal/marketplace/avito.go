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

	// Авито прячет JSON с данными в window.__initialData__ (URL-encoded)
	// Ищем этот кусок скрипта
	jsonPattern := regexp.MustCompile(`window\.__initialData__\s*=\s*"([^"]+)"`)
	jsonMatch := jsonPattern.FindStringSubmatch(html)

	if len(jsonMatch) < 2 {
		log.Printf("[AVITO] Failed to find __initialData__")
		return products
	}

	// Декодируем URL-encoded строку
	decodedJSON, err := url.QueryUnescape(jsonMatch[1])
	if err != nil {
		log.Printf("[AVITO] Failed to unescape JSON: %v", err)
		return products
	}

	// Чтобы не строить сложную структуру, вытаскиваем нужные данные простыми регулярками прямо из JSON!
	// Ищем блоки с объявлениями (начинаются с "url":"/...)
	itemPattern := regexp.MustCompile(`"url":"(/[^"]+_([0-9]{8,}))".*?"title":"([^"]+)".*?"price":\{"value":([0-9]+)`)
	matches := itemPattern.FindAllStringSubmatch(decodedJSON, limit*2)

	seen := make(map[string]bool)

	for _, match := range matches {
		if len(products) >= limit {
			break
		}

		rawURL := match[1]
		idMatch := match[2]
		name := match[3]
		priceStr := match[4]

		// Игнорируем дубликаты
		if seen[idMatch] {
			continue
		}
		seen[idMatch] = true

		fullURL := "https://www.avito.ru" + rawURL

		// Авито отдает цену сразу числом
		price := extractPrice(priceStr)

		// Ищем в названии признаки нового товара
		condition := "Б/У"
		lowerName := strings.ToLower(name)
		if strings.Contains(lowerName, "нов") || strings.Contains(lowerName, "запечатан") {
			condition = "Новое"
		}

		products = append(products, Product{
			ID:          idMatch,
			Name:        cleanString(name),
			Price:       price,
			URL:         fullURL,
			Marketplace: "Avito",
			Condition:   condition,
			InStock:     true,
		})
	}

	log.Printf("[AVITO] Parsed %d products from JSON", len(products))
	return products
}

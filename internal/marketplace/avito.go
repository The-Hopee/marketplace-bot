// internal/marketplace/avito.go
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
	if a.ScraperAPIKey == "" {
		return nil, fmt.Errorf("scraperapi key is missing")
	}

	// Поиск по всей России
	targetURL := fmt.Sprintf("https://www.avito.ru/all?q=%s", url.QueryEscape(query))

	// Для Авито обязательно premium прокси и RU гео
	scraperURL := fmt.Sprintf("http://api.scraperapi.com/?api_key=%s&url=%s&premium=true&country_code=ru",
		a.ScraperAPIKey, url.QueryEscape(targetURL))

	log.Printf("[AVITO] Sending request via ScraperAPI: %s", targetURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scraperURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scraperapi avito request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)
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

	// Ищем карточки объявлений (Avito использует data-marker="item")
	// Вытаскиваем сразу кусок HTML, относящийся к одной карточке
	cardPattern := regexp.MustCompile(`(?s)data-marker="item"(.*?)data-marker="item-photo"`)
	cards := cardPattern.FindAllStringSubmatch(html, limit*2)

	for _, card := range cards {
		if len(products) >= limit {
			break
		}

		cardHTML := card[1]

		// Извлекаем ссылку и название (из атрибута title внутри a)
		linkPattern := regexp.MustCompile(`href="(/[^"]+_([0-9]+))"[^>]*title="([^"]+)"`)
		linkMatch := linkPattern.FindStringSubmatch(cardHTML)

		if len(linkMatch) < 4 {
			continue
		}

		rawURL := linkMatch[1]
		idMatch := linkMatch[2]
		name := cleanString(linkMatch[3]) // Очищаем от HTML-сущностей

		if seen[idMatch] {
			continue
		}
		seen[idMatch] = true

		fullURL := "https://www.avito.ru" + rawURL

		// Извлекаем цену (ищем атрибут content у meta itemprop="price")
		price := float64(0)
		pricePattern := regexp.MustCompile(`meta\s+itemprop="price"\s+content="([0-9]+)"`)
		priceMatch := pricePattern.FindStringSubmatch(cardHTML)
		if len(priceMatch) > 1 {
			price = extractPrice(priceMatch[1])
		}

		// Определяем состояние (Новое или Б/У)
		condition := "Б/У" // По умолчанию на Авито продают б/у
		lowerName := strings.ToLower(name)
		lowerCard := strings.ToLower(cardHTML)

		// Проверяем по ключевым словам в названии и теле карточки
		if strings.Contains(lowerName, "нов") || strings.Contains(lowerName, "запечатан") || strings.Contains(lowerCard, "состояние: новое") {
			condition = "Новое"
		}

		products = append(products, Product{
			ID:          idMatch,
			Name:        name,
			Price:       price,
			URL:         fullURL,
			Marketplace: "Avito",
			Condition:   condition,
			InStock:     true,
		})
	}

	log.Printf("[AVITO] Parsed %d products", len(products))
	return products
}

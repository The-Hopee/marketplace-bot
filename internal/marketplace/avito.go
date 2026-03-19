package marketplace

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"

	"marketplace-bot/internal/browser"
)

type AvitoMarketplace struct {
	browser *browser.Browser
}

func NewAvito(scraperAPIKey string) *AvitoMarketplace {
	return &AvitoMarketplace{
		browser: browser.GetBrowser(),
	}
}

func (a *AvitoMarketplace) GetName() string {
	return "Avito"
}

func (a *AvitoMarketplace) Search(ctx context.Context, query string, limit int) (*SearchResult, error) {
	// Ищем через Яндекс!
	yandexQuery := fmt.Sprintf("%s купить site:avito.ru", query)
	targetURL := fmt.Sprintf("https://yandex.ru/search/?text=%s", url.QueryEscape(yandexQuery))

	log.Printf("[AVITO] Searching via Yandex: %s", targetURL)

	html, err := a.browser.GetPage(ctx, targetURL)
	if err != nil {
		return nil, fmt.Errorf("failed to load yandex page: %w", err)
	}

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

	itemPattern := regexp.MustCompile(`(?s)<li[^>]*class="[^"]*serp-item[^"]*"[^>]*>(.*?)</li>`)
	items := itemPattern.FindAllStringSubmatch(html, limit*3)

	for _, item := range items {
		if len(products) >= limit {
			break
		}
		itemHTML := item[1]

		// Ищем ссылку на объявление Авито (содержит _ и цифры в конце)
		linkPattern := regexp.MustCompile(`href="(https?://(?:www\.)?avito\.ru/[^"]+_([0-9]{8,}))"`)
		linkMatch := linkPattern.FindStringSubmatch(itemHTML)
		if len(linkMatch) < 3 {
			continue
		}

		fullURL := linkMatch[1]
		idMatch := linkMatch[2]

		if seen[idMatch] {
			continue
		}
		seen[idMatch] = true

		name := "Товар Авито"
		namePattern := regexp.MustCompile(`(?s)<h2[^>]*>.*?<span[^>]*>(.*?)</span>`)
		if nameMatch := namePattern.FindStringSubmatch(itemHTML); len(nameMatch) > 1 {
			name = cleanString(nameMatch[1])
		}

		price := float64(0)
		pricePattern := regexp.MustCompile(`(?:от\s*)?([0-9\s\x{00A0}]+)(?:₽|руб)`)
		if priceMatch := pricePattern.FindStringSubmatch(itemHTML); len(priceMatch) > 1 {
			price = extractPrice(priceMatch[1])
		}
		if price == 0 {
			fallbackPattern := regexp.MustCompile(`([0-9]{1,3}(?:\s[0-9]{3})+)\s*₽`)
			if fbMatch := fallbackPattern.FindStringSubmatch(itemHTML); len(fbMatch) > 1 {
				price = extractPrice(fbMatch[1])
			}
		}

		condition := "Б/У"
		lowerText := strings.ToLower(itemHTML)
		if strings.Contains(lowerText, "нов") || strings.Contains(lowerText, "запечатан") {
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

	log.Printf("[AVITO] Parsed %d products from Yandex", len(products))
	return products
}

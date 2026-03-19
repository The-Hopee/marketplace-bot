package marketplace

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"

	"marketplace-bot/internal/browser"
)

type OzonMarketplace struct {
	browser *browser.Browser
}

func NewOzon(scraperAPIKey string) *OzonMarketplace {
	// ScraperAPI больше не нужен, используем твой браузер!
	return &OzonMarketplace{
		browser: browser.GetBrowser(),
	}
}

func (o *OzonMarketplace) GetName() string {
	return "OZON"
}

func (o *OzonMarketplace) Search(ctx context.Context, query string, limit int) (*SearchResult, error) {
	// Ищем через Яндекс!
	yandexQuery := fmt.Sprintf("%s site:ozon.ru/product/", query)
	targetURL := fmt.Sprintf("https://yandex.ru/search/?text=%s", url.QueryEscape(yandexQuery))

	log.Printf("[OZON] Searching via Yandex: %s", targetURL)

	html, err := o.browser.GetPage(ctx, targetURL)
	if err != nil {
		return nil, fmt.Errorf("failed to load yandex page: %w", err)
	}

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

	// Ищем органическую выдачу Яндекса (блок с результатами)
	// Обычно каждый результат лежит в теге <li class="serp-item">
	itemPattern := regexp.MustCompile(`(?s)<li[^>]*class="[^"]*serp-item[^"]*"[^>]*>(.*?)</li>`)
	items := itemPattern.FindAllStringSubmatch(html, limit*3)

	for _, item := range items {
		if len(products) >= limit {
			break
		}
		itemHTML := item[1]

		// Ищем ссылку на Озон
		linkPattern := regexp.MustCompile(`href="(https?://(?:www\.)?ozon\.ru/product/([^"/]+-[0-9]+)[^"]*)"`)
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

		// Название из тега <h2... или <span... с классом title
		name := "Товар Ozon"
		namePattern := regexp.MustCompile(`(?s)<h2[^>]*>.*?<span[^>]*>(.*?)</span>`)
		if nameMatch := namePattern.FindStringSubmatch(itemHTML); len(nameMatch) > 1 {
			name = cleanString(nameMatch[1])
		}

		// Ищем цену в сниппете (Яндекс часто пишет цены типа "от 45 000 ₽")
		price := float64(0)
		pricePattern := regexp.MustCompile(`(?:от\s*)?([0-9\s\x{00A0}]+)(?:₽|руб)`)
		if priceMatch := pricePattern.FindStringSubmatch(itemHTML); len(priceMatch) > 1 {
			price = extractPrice(priceMatch[1])
		}

		// Если цена 0, пытаемся найти любые цифры с пробелом перед символом ₽
		if price == 0 {
			fallbackPattern := regexp.MustCompile(`([0-9]{1,3}(?:\s[0-9]{3})+)\s*₽`)
			if fbMatch := fallbackPattern.FindStringSubmatch(itemHTML); len(fbMatch) > 1 {
				price = extractPrice(fbMatch[1])
			}
		}

		products = append(products, Product{
			ID:          idMatch,
			Name:        name,
			Price:       price,
			URL:         fullURL,
			Marketplace: "OZON",
			InStock:     true,
			Condition:   "Новое",
		})
	}

	log.Printf("[OZON] Parsed %d products from Yandex", len(products))
	return products
}

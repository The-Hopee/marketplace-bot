// internal/marketplace/wildberries.go
package marketplace

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

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
	w.saveDebug(query, html)

	products := w.parseHTML(html, limit)
	log.Printf("[WB] Parsed %d products", len(products))

	return &SearchResult{
		Products:   products,
		TotalCount: len(products),
		Query:      query,
	}, nil
}

func (w *WildberriesMarketplace) saveDebug(query, html string) {
	filename := fmt.Sprintf("/tmp/wb_%s_%d.html",
		strings.ReplaceAll(query, " ", "_"),
		time.Now().Unix(),
	)
	os.WriteFile(filename, []byte(html), 0644)
	log.Printf("[WB] Debug HTML saved to: %s", filename)
}

func (w *WildberriesMarketplace) parseHTML(html string, limit int) []Product {
	var products []Product

	// Находим все карточки товаров
	cardPattern := regexp.MustCompile(`(?s)<article[^>]*data-nm-id="(\d+)"[^>]*>(.*?)</article>`)
	cards := cardPattern.FindAllStringSubmatch(html, limit*2)

	log.Printf("[WB] Found %d product cards", len(cards))

	for _, card := range cards {
		if len(products) >= limit {
			break
		}
		if len(card) < 3 {
			continue
		}

		id := card[1]
		cardHTML := card[2]

		product := w.parseCard(id, cardHTML)
		if product != nil {
			products = append(products, *product)
			log.Printf("[WB] Found: %s - %.0f₽ (старая: %.0f₽, скидка: %d%%)",
				truncate(product.Name, 40), product.Price, product.OldPrice, product.Discount)
		}
	}

	return products
}

func (w *WildberriesMarketplace) parseCard(id, cardHTML string) *Product {
	// Актуальная цена (со скидкой) - ищем price__lower-price с любыми доп. классами
	// <ins class="price__lower-price red-price">63&nbsp;478&nbsp;₽</ins>
	pricePattern := regexp.MustCompile(`<ins[^>]*price__lower-price[^>]*>([^<]+)</ins>`)
	priceMatch := pricePattern.FindStringSubmatch(cardHTML)

	price := 0.0
	if len(priceMatch) >= 2 {
		price = extractPrice(priceMatch[1])
		log.Printf("[WB] Price match: %s -> %.0f", priceMatch[1], price)
	}

	// Старая цена (без скидки) - <del>68&nbsp;184&nbsp;₽</del>
	oldPricePattern := regexp.MustCompile(`<del>([^<]*₽[^<]*)</del>`)
	oldPriceMatch := oldPricePattern.FindStringSubmatch(cardHTML)

	oldPrice := 0.0
	if len(oldPriceMatch) >= 2 {
		oldPrice = extractPrice(oldPriceMatch[1])
	}

	// Скидка - <span class="percentage-sale">−6%</span> или product-card__tip--sale
	discountPattern := regexp.MustCompile(`(?:percentage-sale|tip--sale)[^>]*>[−-]?(\d+)%`)
	discountMatch := discountPattern.FindStringSubmatch(cardHTML)

	discount := 0
	if len(discountMatch) >= 2 {
		discount, _ = strconv.Atoi(discountMatch[1])
	}

	// Название из aria-label
	namePattern := regexp.MustCompile(`aria-label="([^"]+)"`)
	nameMatch := namePattern.FindStringSubmatch(cardHTML)

	name := ""
	if len(nameMatch) >= 2 {
		name = strings.TrimSpace(nameMatch[1])
	}

	// Бренд
	brandPattern := regexp.MustCompile(`<span[^>]*class="product-card__brand"[^>]*>([^<]+)</span>`)
	brandMatch := brandPattern.FindStringSubmatch(cardHTML)

	brand := ""
	if len(brandMatch) >= 2 {
		brand = strings.TrimSpace(brandMatch[1])
	}
	// Если название пустое
	if name == "" {
		// Пробуем взять из alt картинки
		altPattern := regexp.MustCompile(`alt="([^"]+)"`)
		altMatch := altPattern.FindStringSubmatch(cardHTML)
		if len(altMatch) >= 2 {
			name = strings.TrimSpace(altMatch[1])
		}
	}

	if name == "" {
		name = fmt.Sprintf("Товар #%s", id)
	}

	// Добавляем бренд если его нет в названии
	if brand != "" && !strings.Contains(name, brand) {
		name = brand + " / " + name
	}

	return &Product{
		ID:          id,
		Name:        cleanString(name),
		Price:       price,
		OldPrice:    oldPrice,
		Discount:    discount,
		URL:         fmt.Sprintf("https://www.wildberries.ru/catalog/%s/detail.aspx", id),
		Marketplace: "Wildberries",
		InStock:     true,
	}
}

func extractPrice(s string) float64 {
	// Заменяем все виды пробелов
	s = strings.ReplaceAll(s, "&nbsp;", "")
	s = strings.ReplaceAll(s, "\u00a0", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "₽", "")
	s = strings.ReplaceAll(s, "руб", "")
	s = strings.TrimSpace(s)

	// Извлекаем только цифры
	re := regexp.MustCompile(`\d+`)
	matches := re.FindAllString(s, -1)

	if len(matches) == 0 {
		return 0
	}

	numStr := strings.Join(matches, "")
	price, _ := strconv.ParseFloat(numStr, 64)

	return price
}

func cleanString(s string) string {
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "\\u0026", "&")
	s = strings.ReplaceAll(s, "\\u0027", "'")
	s = strings.ReplaceAll(s, "\\\"", "\"")
	s = strings.ReplaceAll(s, "\\/", "/")
	s = strings.ReplaceAll(s, "\\n", " ")
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	return s
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

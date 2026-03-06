// internal/imagesearch/search.go
package imagesearch

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
	"marketplace-bot/internal/marketplace"
)

type ImageSearcher struct {
	browser *browser.Browser
}

func NewImageSearcher() *ImageSearcher {
	return &ImageSearcher{
		browser: browser.GetBrowser(),
	}
}

type ImageSearchResult struct {
	Query    string                `json:"query"`
	Products []marketplace.Product `json:"products"`
	Success  bool                  `json:"success"`
}

func (s *ImageSearcher) SearchByImageURL(ctx context.Context, imageURL string) (*ImageSearchResult, error) {
	log.Printf("[ImageSearch] Searching by image: %s", imageURL)

	searchURL := fmt.Sprintf("https://yandex.ru/images/search?rpt=imageview&url=%s",
		url.QueryEscape(imageURL))

	html, err := s.browser.GetPage(ctx, searchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to load yandex images: %w", err)
	}

	log.Printf("[ImageSearch] Page loaded, size: %d bytes", len(html))
	s.saveDebug("image", html)

	products := s.parseProducts(html)

	result := &ImageSearchResult{
		Products: products,
		Success:  len(products) > 0,
	}

	if len(products) > 0 {
		result.Query = extractQueryFromName(products[0].Name)
	}

	log.Printf("[ImageSearch] Found %d products (WB + OZON)", len(products))

	return result, nil
}

func (s *ImageSearcher) saveDebug(prefix string, html string) {
	filename := fmt.Sprintf("/tmp/yandex_%s_%d.html", prefix, time.Now().Unix())
	os.WriteFile(filename, []byte(html), 0644)
	log.Printf("[ImageSearch] Debug HTML saved to: %s", filename)
}

func (s *ImageSearcher) parseProducts(html string) []marketplace.Product {
	var products []marketplace.Product
	seen := make(map[string]bool)

	// === WILDBERRIES ===
	wbProducts := s.parseWildberries(html, seen)
	products = append(products, wbProducts...)
	log.Printf("[ImageSearch] WB products: %d", len(wbProducts))

	// === OZON ===
	ozonProducts := s.parseOzon(html, seen)
	products = append(products, ozonProducts...)
	log.Printf("[ImageSearch] OZON products: %d", len(ozonProducts))

	// Ограничиваем количество
	if len(products) > 20 {
		products = products[:20]
	}

	return products
}

func (s *ImageSearcher) parseWildberries(html string, seen map[string]bool) []marketplace.Product {
	var products []marketplace.Product

	// Паттерн 1: href потом aria-label
	pattern1 := regexp.MustCompile(`(?s)href="(https?://www\.wildberries\.ru/catalog/(\d+)/[^"]*)"[^>]*aria-label="([^"]+)"`)
	matches := pattern1.FindAllStringSubmatch(html, 30)

	for _, match := range matches {
		if len(match) >= 4 {
			productURL := match[1]
			productID := match[2]
			ariaLabel := match[3]

			key := "wb_" + productID
			if seen[key] {
				continue
			}
			seen[key] = true

			product := parseAriaLabel(ariaLabel, productID, cleanWBURL(productURL), "Wildberries")
			if product != nil {
				products = append(products, *product)
			}
		}
	}

	// Паттерн 2: aria-label потом href
	if len(products) == 0 {
		pattern2 := regexp.MustCompile(`(?s)aria-label="([^"]+)"[^>]*href="(https?://www\.wildberries\.ru/catalog/(\d+)/[^"]*)"`)
		matches2 := pattern2.FindAllStringSubmatch(html, 30)

		for _, match := range matches2 {
			if len(match) >= 4 {
				ariaLabel := match[1]
				productURL := match[2]
				productID := match[3]

				key := "wb_" + productID
				if seen[key] {
					continue
				}
				seen[key] = true

				product := parseAriaLabel(ariaLabel, productID, cleanWBURL(productURL), "Wildberries")
				if product != nil {
					products = append(products, *product)
				}
			}
		}
	}

	return products
}

func (s *ImageSearcher) parseOzon(html string, seen map[string]bool) []marketplace.Product {
	var products []marketplace.Product
	// Паттерн 1: href потом aria-label
	pattern1 := regexp.MustCompile(`(?s)href="(https?://www\.ozon\.ru/product/([a-z0-9-]+)[^"]*)"[^>]*aria-label="([^"]+)"`)
	matches := pattern1.FindAllStringSubmatch(html, 30)

	log.Printf("[ImageSearch] OZON pattern 1 found: %d", len(matches))

	for _, match := range matches {
		if len(match) >= 4 {
			productURL := match[1]
			productSlug := match[2]
			ariaLabel := match[3]

			key := "ozon_" + productSlug
			if seen[key] {
				continue
			}
			seen[key] = true

			product := parseAriaLabel(ariaLabel, productSlug, cleanOzonURL(productURL), "OZON")
			if product != nil {
				products = append(products, *product)
				log.Printf("[ImageSearch] OZON found: %s - %.0f₽", truncate(product.Name, 40), product.Price)
			}
		}
	}

	// Паттерн 2: aria-label потом href
	pattern2 := regexp.MustCompile(`(?s)aria-label="([^"]+)"[^>]*href="(https?://www\.ozon\.ru/product/([a-z0-9-]+)[^"]*)"`)
	matches2 := pattern2.FindAllStringSubmatch(html, 30)

	log.Printf("[ImageSearch] OZON pattern 2 found: %d", len(matches2))

	for _, match := range matches2 {
		if len(match) >= 4 {
			ariaLabel := match[1]
			productURL := match[2]
			productSlug := match[3]

			key := "ozon_" + productSlug
			if seen[key] {
				continue
			}
			seen[key] = true

			product := parseAriaLabel(ariaLabel, productSlug, cleanOzonURL(productURL), "OZON")
			if product != nil {
				products = append(products, *product)
			}
		}
	}

	// Паттерн 3: Просто ссылки на OZON без aria-label
	if len(products) == 0 {
		pattern3 := regexp.MustCompile(`href="(https?://www\.ozon\.ru/product/([a-z0-9-]+-(\d+))[^"]*)"`)
		matches3 := pattern3.FindAllStringSubmatch(html, 20)

		for _, match := range matches3 {
			if len(match) >= 4 {
				productURL := match[1]
				productSlug := match[2]

				key := "ozon_" + productSlug
				if seen[key] {
					continue
				}
				seen[key] = true

				// Пытаемся найти название рядом
				name := findNearbyText(html, productSlug)
				if name == "" {
					name = formatSlugAsName(productSlug)
				}

				products = append(products, marketplace.Product{
					ID:          productSlug,
					Name:        name,
					URL:         cleanOzonURL(productURL),
					Marketplace: "OZON",
					InStock:     true,
				})
			}
		}
	}

	return products
}

func parseAriaLabel(ariaLabel, productID, productURL, marketplaceName string) *marketplace.Product {
	product := &marketplace.Product{
		ID:          productID,
		URL:         productURL,
		Marketplace: marketplaceName,
		InStock:     true,
	}

	// Название (до "Цена" или первой точки с числом)
	namePattern := regexp.MustCompile(`^([^.]+?)(?:\.\s*Цена|\.\s*\d)`)
	if match := namePattern.FindStringSubmatch(ariaLabel); len(match) > 1 {
		product.Name = strings.TrimSpace(match[1])
	} else {
		parts := strings.SplitN(ariaLabel, ".", 2)
		product.Name = strings.TrimSpace(parts[0])
	}

	// Цена
	pricePattern := regexp.MustCompile(`Цена\s*([\d\s]+)₽`)
	if match := pricePattern.FindStringSubmatch(ariaLabel); len(match) > 1 {
		product.Price = parsePrice(match[1])
	}

	// Старая цена
	oldPricePattern := regexp.MustCompile(`Старая цена\s*([\d\s]+)₽`)
	if match := oldPricePattern.FindStringSubmatch(ariaLabel); len(match) > 1 {
		product.OldPrice = parsePrice(match[1])
	}

	// Скидка
	discountPattern := regexp.MustCompile(`-(\d+)%`)
	if match := discountPattern.FindStringSubmatch(ariaLabel); len(match) > 1 {
		product.Discount, _ = strconv.Atoi(match[1])
	}

	if product.Name == "" || len(product.Name) < 3 {
		return nil
	}

	return product
}

func findNearbyText(html, productID string) string {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?s).{0,300}%s.{0,300}`, regexp.QuoteMeta(productID)))
	match := pattern.FindString(html)

	if match == "" {
		return ""
	}
	// Ищем title или aria-label
	titlePattern := regexp.MustCompile(`(?:title|aria-label)="([^"]{5,100})"`)
	if m := titlePattern.FindStringSubmatch(match); len(m) > 1 {
		parts := strings.SplitN(m[1], ".", 2)
		return strings.TrimSpace(parts[0])
	}

	return ""
}

func formatSlugAsName(slug string) string {
	// Преобразуем "iphone-15-pro-256gb-123456" в "Iphone 15 pro 256gb"
	// Убираем ID в конце
	parts := strings.Split(slug, "-")
	if len(parts) > 1 {
		// Убираем последний элемент если это число
		last := parts[len(parts)-1]
		if _, err := strconv.Atoi(last); err == nil {
			parts = parts[:len(parts)-1]
		}
	}

	name := strings.Join(parts, " ")
	if len(name) > 0 {
		// Первую букву в верхний регистр
		runes := []rune(name)
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		name = string(runes)
	}

	return name
}

func cleanWBURL(rawURL string) string {
	re := regexp.MustCompile(`/catalog/(\d+)/`)
	if match := re.FindStringSubmatch(rawURL); len(match) > 1 {
		return fmt.Sprintf("https://www.wildberries.ru/catalog/%s/detail.aspx", match[1])
	}
	return rawURL
}

func cleanOzonURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.RawQuery = ""
	return u.String()
}

func parsePrice(s string) float64 {
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\u00a0", "")
	price, _ := strconv.ParseFloat(s, 64)
	return price
}

func extractQueryFromName(name string) string {
	words := strings.Fields(name)
	if len(words) > 3 {
		words = words[:3]
	}
	return strings.Join(words, " ")
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

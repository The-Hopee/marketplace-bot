package parser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"marketplace-bot/internal/models"
)

type YandexParser struct {
	client *http.Client
}

func NewYandexParser(proxyURL string) *YandexParser {
	transport := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	if proxyURL != "" {
		if proxyParsed, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxyParsed)
		}
	}

	return &YandexParser{
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

func (y *YandexParser) Name() string {
	return "Yandex Market"
}

func (y *YandexParser) Search(ctx context.Context, query string, limit int) ([]models.Product, error) {
	time.Sleep(time.Duration(500+rand.Intn(1000)) * time.Millisecond)

	// API Яндекс Маркета
	searchURL := "https://market.yandex.ru/api/search"

	params := url.Values{}
	params.Set("text", query)
	params.Set("cvredirect", "0")
	params.Set("onstock", "1")
	params.Set("local-offers-first", "0")

	fullURL := searchURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("создание запроса Yandex: %w", err)
	}

	ua := UserAgents[rand.Intn(len(UserAgents))]
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9")
	req.Header.Set("Referer", "https://market.yandex.ru/")
	req.Header.Set("Origin", "https://market.yandex.ru")
	req.Header.Set("Sec-Ch-Ua", `"Not_A Brand";v="8", "Chromium";v="120"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	var resp *http.Response
	var lastErr error

	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(2<<attempt) * time.Second)
		}

		resp, lastErr = y.client.Do(req)
		if lastErr != nil {
			continue
		}

		if resp.StatusCode == 200 {
			break
		}

		resp.Body.Close()
		lastErr = fmt.Errorf("Yandex вернул статус: %d", resp.StatusCode)
	}

	if lastErr != nil {
		// Попробуем альтернативный метод через HTML парсинг
		return y.searchViaHTML(ctx, query, limit)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("чтение ответа Yandex: %w", err)
	}

	return y.parseYandexResponse(body, limit)
}

func (y *YandexParser) searchViaHTML(ctx context.Context, query string, limit int) ([]models.Product, error) {
	searchURL := fmt.Sprintf("https://market.yandex.ru/search?text=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	ua := UserAgents[rand.Intn(len(UserAgents))]
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9")

	resp, err := y.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return y.parseHTMLResponse(string(body), limit)
}

func (y *YandexParser) parseYandexResponse(body []byte, limit int) ([]models.Product, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("парсинг JSON Yandex: %w", err)
	}

	products := make([]models.Product, 0, limit)

	// Ищем в структуре ответа
	collections, ok := result["collections"].(map[string]interface{})
	if !ok {
		return products, nil
	}
	items, ok := collections["offer"].([]interface{})
	if !ok {
		// Попробуем другой путь
		items, ok = collections["product"].([]interface{})
		if !ok {
			return products, nil
		}
	}

	for i, item := range items {
		if i >= limit {
			break
		}

		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		product := models.Product{
			Marketplace: "Yandex Market",
			InStock:     true,
		}

		if id, ok := itemMap["id"].(float64); ok {
			product.ID = fmt.Sprintf("%.0f", id)
		} else if id, ok := itemMap["id"].(string); ok {
			product.ID = id
		}

		if title, ok := itemMap["title"].(string); ok {
			product.Name = title
		} else if titles, ok := itemMap["titles"].(map[string]interface{}); ok {
			if raw, ok := titles["raw"].(string); ok {
				product.Name = raw
			}
		}

		if prices, ok := itemMap["prices"].(map[string]interface{}); ok {
			if price, ok := prices["value"].(float64); ok {
				product.Price = price
			}
			if oldPrice, ok := prices["base"].(float64); ok {
				product.OldPrice = oldPrice
			}
		}

		if rating, ok := itemMap["rating"].(map[string]interface{}); ok {
			if value, ok := rating["value"].(float64); ok {
				product.Rating = value
			}
			if count, ok := rating["count"].(float64); ok {
				product.ReviewCount = int(count)
			}
		}

		if slug, ok := itemMap["slug"].(string); ok {
			product.URL = fmt.Sprintf("https://market.yandex.ru/product/%s/%s", product.ID, slug)
		} else {
			product.URL = fmt.Sprintf("https://market.yandex.ru/product/%s", product.ID)
		}

		if photos, ok := itemMap["photos"].([]interface{}); ok && len(photos) > 0 {
			if photo, ok := photos[0].(map[string]interface{}); ok {
				if url, ok := photo["url"].(string); ok {
					product.ImageURL = url
				}
			}
		}

		if product.Name != "" && product.ID != "" {
			products = append(products, product)
		}
	}

	return products, nil
}

func (y *YandexParser) parseHTMLResponse(html string, limit int) ([]models.Product, error) {
	products := make([]models.Product, 0, limit)

	// Ищем JSON данные в HTML (Yandex обычно встраивает их в script теги)
	re := regexp.MustCompile(`"searchResults":\s*(\{[^}]+\})`)
	matches := re.FindStringSubmatch(html)

	if len(matches) < 2 {
		// Пробуем найти данные о товарах через другой паттерн
		productRe := regexp.MustCompile(`data-zone-name="snippet-card"[^>]*>`)
		productMatches := productRe.FindAllString(html, limit)

		// Базовый парсинг, если API не работает
		priceRe := regexp.MustCompile(`"price":\s*(\d+)`)
		titleRe := regexp.MustCompile(`"title":\s*"([^"]+)"`)

		priceMatches := priceRe.FindAllStringSubmatch(html, limit)
		titleMatches := titleRe.FindAllStringSubmatch(html, limit)

		for i := 0; i < len(priceMatches) && i < len(titleMatches) && i < limit; i++ {
			price, _ := strconv.ParseFloat(priceMatches[i][1], 64)
			products = append(products, models.Product{
				ID:          fmt.Sprintf("ym_%d", i),
				Name:        titleMatches[i][1],
				Price:       price,
				Marketplace: "Yandex Market",
				InStock:     true,
			})
		}

		return products, nil
	}

	return products, nil
}

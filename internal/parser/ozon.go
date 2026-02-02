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
	"strings"
	"time"

	"marketplace-bot/internal/models"
)

type OzonParser struct {
	client *http.Client
}

func NewOzonParser(proxyURL string) *OzonParser {
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

	return &OzonParser{
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

func (o *OzonParser) Name() string {
	return "Ozon"
}

type ozonSearchResponse struct {
	Items []ozonItem `json:"items"`
}

type ozonItem struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Link     string `json:"link"`
	Price    string `json:"price"`
	OldPrice string `json:"oldPrice"`
	Rating   string `json:"rating"`
	Reviews  int    `json:"reviews"`
	Image    string `json:"image"`
}

func (o *OzonParser) Search(ctx context.Context, query string, limit int) ([]models.Product, error) {
	time.Sleep(time.Duration(500+rand.Intn(1000)) * time.Millisecond)

	// Используем API через layout компонент
	searchURL := "https://www.ozon.ru/api/entrypoint-api.bx/page/json/v2"

	params := url.Values{}
	params.Set("url", fmt.Sprintf("/search/?text=%s&from_global=true", url.QueryEscape(query)))

	fullURL := searchURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("создание запроса Ozon: %w", err)
	}

	ua := UserAgents[rand.Intn(len(UserAgents))]
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9")
	req.Header.Set("Referer", "https://www.ozon.ru/")
	req.Header.Set("Origin", "https://www.ozon.ru")
	req.Header.Set("Sec-Ch-Ua", `"Not_A Brand";v="8", "Chromium";v="120"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", "Windows")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")

	// Важные cookies для Ozon
	req.Header.Set("Cookie", "__Secure-access-token=; __Secure-refresh-token=; abt_data=")

	var resp *http.Response
	var lastErr error

	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(2<<attempt) * time.Second)
		}

		resp, lastErr = o.client.Do(req)
		if lastErr != nil {
			continue
		}

		if resp.StatusCode == 200 {
			break
		}

		resp.Body.Close()
		lastErr = fmt.Errorf("Ozon вернул статус: %d", resp.StatusCode)
	}

	if lastErr != nil {
		return nil, lastErr
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("чтение ответа Ozon: %w", err)
	}

	return o.parseOzonResponse(body, limit)
}

func (o *OzonParser) parseOzonResponse(body []byte, limit int) ([]models.Product, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("парсинг JSON Ozon: %w", err)
	}

	products := make([]models.Product, 0, limit)

	// Ozon возвращает сложную структуру, ищем widgetStates
	widgetStates, ok := result["widgetStates"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("не найден widgetStates в ответе Ozon")
	}

	// Ищем ключ, содержащий searchResultsV2
	for key, value := range widgetStates {
		if strings.Contains(key, "searchResultsV2") {
			strValue, ok := value.(string)
			if !ok {
				continue
			}

			var searchData map[string]interface{}
			if err := json.Unmarshal([]byte(strValue), &searchData); err != nil {
				continue
			}

			items, ok := searchData["items"].([]interface{})
			if !ok {
				continue
			}
			for i, item := range items {
				if i >= limit {
					break
				}

				itemMap, ok := item.(map[string]interface{})
				if !ok {
					continue
				}

				product := o.extractProduct(itemMap)
				if product.ID != "" {
					products = append(products, product)
				}
			}
			break
		}
	}

	return products, nil
}

func (o *OzonParser) extractProduct(item map[string]interface{}) models.Product {
	product := models.Product{
		Marketplace: "Ozon",
		InStock:     true,
	}

	// Получаем mainState для основной информации
	if mainState, ok := item["mainState"].([]interface{}); ok {
		for _, state := range mainState {
			stateMap, ok := state.(map[string]interface{})
			if !ok {
				continue
			}

			id, _ := stateMap["id"].(string)
			atom, _ := stateMap["atom"].(map[string]interface{})

			switch id {
			case "name":
				if textAtom, ok := atom["textAtom"].(map[string]interface{}); ok {
					product.Name, _ = textAtom["text"].(string)
				}
			case "atom":
				if priceAtom, ok := atom["price"].(map[string]interface{}); ok {
					if price, ok := priceAtom["price"].(string); ok {
						product.Price = o.parsePrice(price)
					}
					if oldPrice, ok := priceAtom["originalPrice"].(string); ok {
						product.OldPrice = o.parsePrice(oldPrice)
					}
				}
			}
		}
	}

	// ID товара
	if link, ok := item["action"].(map[string]interface{}); ok {
		if url, ok := link["link"].(string); ok {
			product.URL = "https://www.ozon.ru" + url
			// Извлекаем ID из URL
			re := regexp.MustCompile(`/product/[^/]+-(\d+)/`)
			matches := re.FindStringSubmatch(url)
			if len(matches) > 1 {
				product.ID = matches[1]
			}
		}
	}

	// Рейтинг
	if rightState, ok := item["rightState"].([]interface{}); ok {
		for _, state := range rightState {
			stateMap, ok := state.(map[string]interface{})
			if !ok {
				continue
			}
			if atom, ok := stateMap["atom"].(map[string]interface{}); ok {
				if rating, ok := atom["rating"].(map[string]interface{}); ok {
					if val, ok := rating["rating"].(float64); ok {
						product.Rating = val
					}
					if count, ok := rating["count"].(float64); ok {
						product.ReviewCount = int(count)
					}
				}
			}
		}
	}

	// Картинка
	if tileImage, ok := item["tileImage"].(map[string]interface{}); ok {
		if items, ok := tileImage["items"].([]interface{}); ok && len(items) > 0 {
			if imgItem, ok := items[0].(map[string]interface{}); ok {
				if image, ok := imgItem["image"].(map[string]interface{}); ok {
					product.ImageURL, _ = image["link"].(string)
				}
			}
		}
	}

	return product
}

func (o *OzonParser) parsePrice(priceStr string) float64 {
	re := regexp.MustCompile(`[\d\s]+`)
	matches := re.FindString(priceStr)
	matches = strings.ReplaceAll(matches, " ", "")
	matches = strings.ReplaceAll(matches, "\u00a0", "")
	price, _ := strconv.ParseFloat(matches, 64)
	return price
}

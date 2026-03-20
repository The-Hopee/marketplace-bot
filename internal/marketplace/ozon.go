package marketplace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
)

type OzonMarketplace struct {
	XMLRiverURL string // Твой URL от сервиса XMLRiver
}

func NewOzon(xmlRiverURL string) *OzonMarketplace {
	return &OzonMarketplace{
		XMLRiverURL: xmlRiverURL,
	}
}

func (o *OzonMarketplace) GetName() string {
	return "OZON"
}

// XMLRiverResponse описывает структуру ответа сервиса (зависит от того, JSON или XML ты выберешь в настройках)
// Ниже пример для JSON-ответа.
type XMLRiverResponse struct {
	Items []struct {
		URL   string `json:"url"`
		Title string `json:"title"`
		Text  string `json:"text"` // Сниппет (описание)
	} `json:"items"`
}

func (o *OzonMarketplace) Search(ctx context.Context, query string, limit int) (*SearchResult, error) {
	if o.XMLRiverURL == "" {
		return nil, fmt.Errorf("XMLRiver URL is empty")
	}

	yandexQuery := fmt.Sprintf("%s site:ozon.ru/product/", query)

	// Формируем запрос к XMLRiver
	apiURL := fmt.Sprintf("%s&query=%s", o.XMLRiverURL, url.QueryEscape(yandexQuery))

	log.Printf("[OZON] Sending request to XMLRiver: %s", yandexQuery)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
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

	var riverData XMLRiverResponse
	if err := json.Unmarshal(body, &riverData); err != nil {
		log.Printf("[OZON] Failed to parse XMLRiver JSON. Body: %s", string(body))
		return nil, err
	}

	var products []Product
	seen := make(map[string]bool)

	for _, item := range riverData.Items {
		if len(products) >= limit {
			break
		}

		// Достаем ID из ссылки
		linkPattern := regexp.MustCompile(`ozon\.ru/product/([^"/]+-[0-9]+)`)
		linkMatch := linkPattern.FindStringSubmatch(item.URL)
		if len(linkMatch) < 2 {
			continue
		}

		idMatch := linkMatch[1]
		if seen[idMatch] {
			continue
		}
		seen[idMatch] = true

		// Ищем цену в описании (сниппете)
		price := float64(0)
		pricePattern := regexp.MustCompile(`(?:от\s*)?([0-9\s\x{00A0}]+)(?:₽|руб)`)
		if pMatch := pricePattern.FindStringSubmatch(item.Text); len(pMatch) > 1 {
			price = extractPrice(pMatch[1])
		}

		if price == 0 {
			fallbackPattern := regexp.MustCompile(`([0-9]{1,3}(?:\s[0-9]{3})+)\s*₽`)
			if fbMatch := fallbackPattern.FindStringSubmatch(item.Text); len(fbMatch) > 1 {
				price = extractPrice(fbMatch[1])
			}
		}

		products = append(products, Product{
			ID:          idMatch,
			Name:        item.Title,
			Price:       price,
			URL:         item.URL,
			Marketplace: "OZON",
			Condition:   "Новое",
			InStock:     true,
		})
	}

	log.Printf("[OZON] Found %d products via XMLRiver", len(products))

	return &SearchResult{
		Products:   products,
		TotalCount: len(products),
		Query:      query,
	}, nil
}

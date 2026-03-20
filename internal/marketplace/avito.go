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
	"strings"
)

type AvitoMarketplace struct {
	XMLRiverURL string // URL от XMLRiver, который ты прокинешь из конфига
}

func NewAvito(xmlRiverURL string) *AvitoMarketplace {
	return &AvitoMarketplace{
		XMLRiverURL: xmlRiverURL,
	}
}

func (a *AvitoMarketplace) GetName() string {
	return "Avito"
}

func (a *AvitoMarketplace) Search(ctx context.Context, query string, limit int) (*SearchResult, error) {
	if a.XMLRiverURL == "" {
		return nil, fmt.Errorf("XMLRiver URL is empty")
	}

	// Формируем запрос для Яндекса. Добавляем слово "купить", чтобы Яндекс чаще выдавал карточки товаров, а не категории
	yandexQuery := fmt.Sprintf("%s купить site:avito.ru", query)

	// Формируем итоговый URL к XMLRiver
	apiURL := fmt.Sprintf("%s&query=%s", a.XMLRiverURL, url.QueryEscape(yandexQuery))

	log.Printf("[AVITO] Sending request to XMLRiver: %s", yandexQuery)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xmlriver request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var riverData XMLRiverResponse
	// Пытаемся распарсить JSON. Если XMLRiver вернет ошибку (например, баланс кончился), unmarshal упадет.
	if err := json.Unmarshal(body, &riverData); err != nil {
		log.Printf("[AVITO] Failed to parse XMLRiver JSON. Error: %v. Body (first 200 chars): %s", err, string(body)[:min(len(body), 200)])
		return nil, fmt.Errorf("invalid response from xmlriver: %w", err)
	}

	var products []Product
	seen := make(map[string]bool)

	for _, item := range riverData.Items {
		if len(products) >= limit {
			break
		}

		// Ищем ссылку на конкретное объявление Авито (содержит _ и цифры в конце)
		// Пример: https://www.avito.ru/moskva/telefony/iphone_15_pro_max_256gb_358920184
		linkPattern := regexp.MustCompile(`avito\.ru/[^"]+_([0-9]{8,})`)
		linkMatch := linkPattern.FindStringSubmatch(item.URL)

		// Если это ссылка на категорию, а не на товар — пропускаем
		if len(linkMatch) < 2 {
			continue
		}

		idMatch := linkMatch[1]
		if seen[idMatch] {
			continue
		}
		seen[idMatch] = true

		// Пытаемся вытащить цену из сниппета (описания) или заголовка
		price := float64(0)
		textToSearch := item.Title + " " + item.Text

		// Ищем цену вида: 45 000 ₽, 45000 руб, от 45 000₽
		pricePattern := regexp.MustCompile(`(?:от\s*)?([0-9\s\x{00A0}]+)(?:₽|руб)`)
		if pMatch := pricePattern.FindStringSubmatch(textToSearch); len(pMatch) > 1 {
			price = extractPrice(pMatch[1])
		}

		// Резервный поиск цены: просто цифры с пробелами и знаком рубля
		if price == 0 {
			fallbackPattern := regexp.MustCompile(`([0-9]{1,3}(?:\s[0-9]{3})+)\s*₽`)
			if fbMatch := fallbackPattern.FindStringSubmatch(textToSearch); len(fbMatch) > 1 {
				price = extractPrice(fbMatch[1])
			}
		}

		// Пытаемся определить состояние (Новое или Б/У)
		condition := "Б/У" // По умолчанию Авито
		lowerText := strings.ToLower(textToSearch)
		if strings.Contains(lowerText, "состояние: новое") || strings.Contains(lowerText, "новый") || strings.Contains(lowerText, "запечатан") {
			condition = "Новое"
		}

		products = append(products, Product{
			ID:          idMatch,
			Name:        item.Title,
			Price:       price,
			URL:         item.URL,
			Marketplace: "Avito",
			Condition:   condition,
			InStock:     true,
		})
	}

	log.Printf("[AVITO] Found %d products via XMLRiver", len(products))

	return &SearchResult{
		Products:   products,
		TotalCount: len(products),
		Query:      query,
	}, nil
}

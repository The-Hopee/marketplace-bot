package marketplace

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type AvitoMarketplace struct {
	XMLRiverURL string
}

func NewAvito(xmlRiverURL string) *AvitoMarketplace {
	return &AvitoMarketplace{XMLRiverURL: xmlRiverURL}
}

func (a *AvitoMarketplace) GetName() string {
	return "Avito"
}

func (a *AvitoMarketplace) Search(ctx context.Context, query string, limit int, city string) (*SearchResult, error) {
	if a.XMLRiverURL == "" {
		return nil, fmt.Errorf("XMLRiver URL is empty")
	}

	cityPart := ""
	if city != "" {
		cityPart = city + " "
	}

	// Яндекс поставит совпадения с городом на первые места!
	yandexQuery := fmt.Sprintf("%s %sкупить site:avito.ru -inurl:q= -inurl:all", query, cityPart)
	apiURL := fmt.Sprintf("%s&query=%s", a.XMLRiverURL, url.QueryEscape(yandexQuery))

	log.Printf("[AVITO] Sending request to XMLRiver: %s", apiURL)

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var riverData YandexXMLResponse
	if err := xml.Unmarshal(body, &riverData); err != nil {
		log.Printf("[AVITO] XML Parse Error: %v", err)
		return nil, err
	}

	var products []Product
	seen := make(map[string]bool)

	for _, doc := range riverData.Docs {
		if len(products) >= limit {
			break
		}

		// Берем любые ссылки Авито
		linkPattern := regexp.MustCompile(`(?i)avito\.ru/([^"]+)`)
		linkMatch := linkPattern.FindStringSubmatch(doc.URL)
		if len(linkMatch) < 2 {
			continue
		}

		idMatch := linkMatch[1]
		if seen[idMatch] {
			continue
		}
		seen[idMatch] = true

		price := doc.Price
		textToSearch := strings.ReplaceAll(doc.Title+" "+strings.Join(doc.Passages, " "), "&nbsp;", "")

		if price == 0 {
			re := regexp.MustCompile(`([0-9\s]{3,10})\s*(?:₽|руб|р\.)`)
			if matches := re.FindAllStringSubmatch(textToSearch, -1); len(matches) > 0 {
				cleanNum := regexp.MustCompile(`\D`).ReplaceAllString(matches[0][1], "")
				price, _ = strconv.ParseFloat(cleanNum, 64)
			}
		}

		condition := "Б/У"
		lowerText := strings.ToLower(textToSearch)
		if strings.Contains(lowerText, "состояние: новое") || strings.Contains(lowerText, "новый") || strings.Contains(lowerText, "запечатан") {
			condition = "Новое"
		}

		cleanTitle := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(doc.Title, "")
		if cleanTitle == "" {
			cleanTitle = "Товар Авито"
		}

		products = append(products, Product{
			ID: idMatch, Name: cleanString(cleanTitle), Price: price,
			URL: doc.URL, Marketplace: "Avito", Condition: condition, InStock: true,
		})
	}

	log.Printf("[AVITO] Found %d products via XMLRiver", len(products))
	return &SearchResult{Products: products, TotalCount: len(products), Query: query}, nil
}

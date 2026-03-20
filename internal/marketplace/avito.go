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

func (a *AvitoMarketplace) Search(ctx context.Context, query string, limit int) (*SearchResult, error) {
	if a.XMLRiverURL == "" {
		return nil, fmt.Errorf("XMLRiver URL is empty")
	}

	yandexQuery := fmt.Sprintf("%s купить site:avito.ru", query)
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
		log.Printf("[AVITO] Failed to parse XML. Error: %v", err)
		return nil, err
	}

	var products []Product
	seen := make(map[string]bool)

	for _, doc := range riverData.Docs {
		if len(products) >= limit {
			break
		}

		linkPattern := regexp.MustCompile(`avito\.ru/[^"]+_([0-9]{8,})`)
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
		textToSearch := doc.Title + " " + strings.Join(doc.Passages, " ")

		if price == 0 {
			pricePattern := regexp.MustCompile(`(?:от\s*)?([0-9\s\x{00A0}]+)(?:₽|руб)`)
			if pMatch := pricePattern.FindStringSubmatch(textToSearch); len(pMatch) > 1 {
				price = extractPrice(pMatch[1])
			}
			if price == 0 {
				fallbackPattern := regexp.MustCompile(`([0-9]{1,3}(?:\s[0-9]{3})+)\s*₽`)
				if fbMatch := fallbackPattern.FindStringSubmatch(textToSearch); len(fbMatch) > 1 {
					price = extractPrice(fbMatch[1])
				}
			}
		}

		condition := "Б/У"
		lowerText := strings.ToLower(textToSearch)
		if strings.Contains(lowerText, "состояние: новое") || strings.Contains(lowerText, "новый") || strings.Contains(lowerText, "запечатан") {
			condition = "Новое"
		}

		cleanTitle := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(doc.Title, "")

		products = append(products, Product{
			ID: idMatch, Name: cleanString(cleanTitle), Price: price,
			URL: doc.URL, Marketplace: "Avito", Condition: condition, InStock: true,
		})
	}

	log.Printf("[AVITO] Found %d products via XMLRiver", len(products))
	return &SearchResult{Products: products, TotalCount: len(products), Query: query}, nil
}

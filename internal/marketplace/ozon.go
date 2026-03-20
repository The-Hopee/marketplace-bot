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

type OzonMarketplace struct {
	XMLRiverURL string
}

func NewOzon(xmlRiverURL string) *OzonMarketplace {
	return &OzonMarketplace{XMLRiverURL: xmlRiverURL}
}

func (o *OzonMarketplace) GetName() string {
	return "OZON"
}

// ИДЕАЛЬНАЯ СТРУКТУРА ПОД ТВОЙ XML
type YandexXMLResponse struct {
	XMLName xml.Name `xml:"yandexsearch"`
	Docs    []struct {
		URL      string   `xml:"url"`
		Title    string   `xml:"title"`
		Passages []string `xml:"passages>passage"`
		Price    float64  `xml:"price"` // Яндекс сам отдает цену!
	} `xml:"response>results>grouping>group>doc"`
}

func (o *OzonMarketplace) Search(ctx context.Context, query string, limit int) (*SearchResult, error) {
	if o.XMLRiverURL == "" {
		return nil, fmt.Errorf("XMLRiver URL is empty")
	}

	yandexQuery := fmt.Sprintf("%s site:ozon.ru/product/", query)
	apiURL := fmt.Sprintf("%s&query=%s", o.XMLRiverURL, url.QueryEscape(yandexQuery))

	log.Printf("[OZON] Sending request to XMLRiver: %s", apiURL)

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
		log.Printf("[OZON] Failed to parse XML. Error: %v\nBody: %s", err, string(body)[:min(len(body), 500)])
		return nil, err
	}

	var products []Product
	seen := make(map[string]bool)

	for _, doc := range riverData.Docs {
		if len(products) >= limit {
			break
		}

		// Озон ссылки
		linkPattern := regexp.MustCompile(`(?i)ozon\.ru/product/([^"/]+-[0-9]+)`)
		linkMatch := linkPattern.FindStringSubmatch(doc.URL)
		if len(linkMatch) < 2 {
			continue
		}

		idMatch := linkMatch[1]
		if seen[idMatch] {
			continue
		}
		seen[idMatch] = true

		// 1. Берем цену прямо из Яндекса!
		price := doc.Price

		// 2. Если Яндекс не нашел тег price, ищем в тексте
		if price == 0 {
			textToSearch := doc.Title + " " + strings.Join(doc.Passages, " ")
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

		cleanTitle := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(doc.Title, "")

		products = append(products, Product{
			ID: idMatch, Name: cleanString(cleanTitle), Price: price,
			URL: doc.URL, Marketplace: "OZON", Condition: "Новое", InStock: true,
		})
	}

	log.Printf("[OZON] Found %d products via XMLRiver", len(products))
	return &SearchResult{Products: products, TotalCount: len(products), Query: query}, nil
}

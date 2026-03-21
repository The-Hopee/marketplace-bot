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

type OzonMarketplace struct {
	XMLRiverURL string
}

func NewOzon(xmlRiverURL string) *OzonMarketplace {
	return &OzonMarketplace{XMLRiverURL: xmlRiverURL}
}

func (o *OzonMarketplace) GetName() string {
	return "OZON"
}

type YandexXMLResponse struct {
	XMLName xml.Name `xml:"yandexsearch"`
	Docs    []struct {
		URL      string   `xml:"url"`
		Title    string   `xml:"title"`
		Passages []string `xml:"passages>passage"`
		Price    float64  `xml:"price"`
	} `xml:"response>results>grouping>group>doc"`
}

func (o *OzonMarketplace) Search(ctx context.Context, query string, limit int, city string) (*SearchResult, error) {
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
		log.Printf("[OZON] XML Parse Error: %v", err)
		return nil, err
	}

	var products []Product
	seen := make(map[string]bool)

	for _, doc := range riverData.Docs {
		if len(products) >= limit {
			break
		}

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

		price := doc.Price
		if price == 0 {
			textToSearch := strings.ReplaceAll(doc.Title+" "+strings.Join(doc.Passages, " "), "&nbsp;", "")
			re := regexp.MustCompile(`([0-9\s]{3,10})\s*(?:₽|руб|р\.)`)
			if matches := re.FindAllStringSubmatch(textToSearch, -1); len(matches) > 0 {
				cleanNum := regexp.MustCompile(`\D`).ReplaceAllString(matches[0][1], "")
				price, _ = strconv.ParseFloat(cleanNum, 64)
			}
		}

		cleanTitle := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(doc.Title, "")

		// Если Яндекс не отдал Title, достаем его из ссылки
		if cleanTitle == "" || len(cleanTitle) < 3 {
			urlParts := strings.Split(doc.URL, "/")
			for _, part := range urlParts {
				if strings.Contains(part, "-") && len(part) > 10 {
					cleanTitle = strings.ReplaceAll(part, "-", " ")
					cleanTitle = regexp.MustCompile(`\s[0-9]+$`).ReplaceAllString(cleanTitle, "")
					break
				}
			}
		}

		if cleanTitle == "" {
			cleanTitle = "Товар Ozon"
		}

		products = append(products, Product{
			ID: idMatch, Name: cleanString(cleanTitle), Price: price,
			URL: doc.URL, Marketplace: "OZON", Condition: "Новое", InStock: true,
		})
	}

	log.Printf("[OZON] Found %d products via XMLRiver", len(products))
	return &SearchResult{Products: products, TotalCount: len(products), Query: query}, nil
}

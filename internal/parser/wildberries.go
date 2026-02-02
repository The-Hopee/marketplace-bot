package parser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"time"

	"marketplace-bot/internal/models"
)

type WildberriesParser struct {
	client *http.Client
}

func NewWildberriesParser(proxyURL string) *WildberriesParser {
	transport := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  false,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	if proxyURL != "" {
		if proxyParsed, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxyParsed)
		}
	}

	return &WildberriesParser{
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

func (w *WildberriesParser) Name() string {
	return "Wildberries"
}

// WB API Response структуры
type wbSearchResponse struct {
	Data struct {
		Products []wbProduct `json:"products"`
	} `json:"data"`
}

type wbProduct struct {
	ID         int        `json:"id"`
	Name       string     `json:"name"`
	Brand      string     `json:"brand"`
	PriceU     int        `json:"priceU"`
	SalePriceU int        `json:"salePriceU"`
	Rating     float64    `json:"rating"`
	Feedbacks  int        `json:"feedbacks"`
	Volume     int        `json:"volume"`
	Pics       int        `json:"pics"`
	Colors     []struct{} `json:"colors"`
}

func (w *WildberriesParser) Search(ctx context.Context, query string, limit int) ([]models.Product, error) {
	// Добавляем случайную задержку для имитации человека
	time.Sleep(time.Duration(500+rand.Intn(1000)) * time.Millisecond)

	// Используем мобильный API - он более стабильный
	baseURL := "https://search.wb.ru/exactmatch/ru/common/v5/search"

	params := url.Values{}
	params.Set("ab_testing", "false")
	params.Set("appType", "1")
	params.Set("curr", "rub")
	params.Set("dest", "-1257786")
	params.Set("query", query)
	params.Set("resultset", "catalog")
	params.Set("sort", "popular")
	params.Set("spp", "30")
	params.Set("suppressSpellcheck", "false")

	fullURL := baseURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("создание запроса WB: %w", err)
	}

	// Критически важные заголовки для WB
	ua := UserAgents[rand.Intn(len(UserAgents))]
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Origin", "https://www.wildberries.ru")
	req.Header.Set("Referer", "https://www.wildberries.ru/")
	req.Header.Set("Sec-Ch-Ua", `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", "Windows")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")

	var resp *http.Response
	var lastErr error

	// Retry логика с экспоненциальной задержкой
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<attempt) * time.Second
			time.Sleep(delay + time.Duration(rand.Intn(1000))*time.Millisecond)
		}

		resp, lastErr = w.client.Do(req)
		if lastErr != nil {
			continue
		}

		if resp.StatusCode == 200 {
			break
		}

		if resp.StatusCode == 429 {
			resp.Body.Close()
			time.Sleep(5 * time.Second) // Ждём при 429
			continue
		}

		resp.Body.Close()
		lastErr = fmt.Errorf("WB вернул статус: %d", resp.StatusCode)
	}

	if lastErr != nil {
		return nil, lastErr
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("чтение ответа WB: %w", err)
	}

	var searchResp wbSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("парсинг JSON WB: %w", err)
	}

	products := make([]models.Product, 0, limit)
	for i, p := range searchResp.Data.Products {
		if i >= limit {
			break
		}
		price := float64(p.SalePriceU) / 100
		oldPrice := float64(p.PriceU) / 100
		if p.SalePriceU == 0 {
			price = oldPrice
		}

		// Формируем URL картинки
		vol := p.ID / 100000
		part := p.ID / 1000
		host := w.getImageHost(vol)
		imageURL := fmt.Sprintf("https://%s/vol%d/part%d/%d/images/c516x688/1.webp", host, vol, part, p.ID)

		products = append(products, models.Product{
			ID:          fmt.Sprintf("%d", p.ID),
			Name:        fmt.Sprintf("%s %s", p.Brand, p.Name),
			Price:       price,
			OldPrice:    oldPrice,
			Rating:      p.Rating,
			ReviewCount: p.Feedbacks,
			URL:         fmt.Sprintf("https://www.wildberries.ru/catalog/%d/detail.aspx", p.ID),
			ImageURL:    imageURL,
			Marketplace: "Wildberries",
			InStock:     true,
		})
	}

	return products, nil
}

func (w *WildberriesParser) getImageHost(vol int) string {
	switch {
	case vol <= 143:
		return "basket-01.wbbasket.ru"
	case vol <= 287:
		return "basket-02.wbbasket.ru"
	case vol <= 431:
		return "basket-03.wbbasket.ru"
	case vol <= 719:
		return "basket-04.wbbasket.ru"
	case vol <= 1007:
		return "basket-05.wbbasket.ru"
	case vol <= 1061:
		return "basket-06.wbbasket.ru"
	case vol <= 1115:
		return "basket-07.wbbasket.ru"
	case vol <= 1169:
		return "basket-08.wbbasket.ru"
	case vol <= 1313:
		return "basket-09.wbbasket.ru"
	case vol <= 1601:
		return "basket-10.wbbasket.ru"
	case vol <= 1655:
		return "basket-11.wbbasket.ru"
	case vol <= 1919:
		return "basket-12.wbbasket.ru"
	case vol <= 2045:
		return "basket-13.wbbasket.ru"
	case vol <= 2189:
		return "basket-14.wbbasket.ru"
	case vol <= 2405:
		return "basket-15.wbbasket.ru"
	case vol <= 2621:
		return "basket-16.wbbasket.ru"
	default:
		return "basket-17.wbbasket.ru"
	}
}

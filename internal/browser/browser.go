package browser

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

type Browser struct {
	browser *rod.Browser
	mu      sync.Mutex
}

var (
	instance *Browser
	once     sync.Once
)

// Singleton - один браузер на всё приложение
func GetBrowser() *Browser {
	once.Do(func() {
		instance = &Browser{}
		instance.init()
	})
	return instance
}

func (b *Browser) init() {
	// Путь к Chrome (в Docker будет /usr/bin/chromium-browser)
	path, exists := launcher.LookPath()
	if !exists {
		path = "/usr/bin/chromium-browser"
	}

	log.Printf("[Browser] Using Chrome at: %s", path)

	u := launcher.New().
		Bin(path).
		Headless(true).
		Set("disable-gpu").
		Set("no-sandbox").
		Set("disable-dev-shm-usage").
		Set("disable-setuid-sandbox").
		Set("disable-web-security").
		Set("disable-features", "IsolateOrigins,site-per-process").
		MustLaunch()

	b.browser = rod.New().ControlURL(u).MustConnect()
	log.Println("[Browser] Chrome started successfully")
}

func (b *Browser) GetPage(ctx context.Context, url string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	page := b.browser.MustPage()
	defer page.MustClose()

	// Устанавливаем User-Agent
	page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
	})

	// Переходим на страницу
	err := rod.Try(func() {
		page.Timeout(30 * time.Second).MustNavigate(url).MustWaitLoad()
	})
	if err != nil {
		return "", err
	}

	// Ждём немного для динамического контента
	time.Sleep(2 * time.Second)

	// Скроллим для загрузки товаров
	page.Mouse.Scroll(0, 1000, 1)
	time.Sleep(1 * time.Second)

	html, err := page.HTML()
	if err != nil {
		return "", err
	}

	return html, nil
}

func (b *Browser) Close() {
	if b.browser != nil {
		b.browser.MustClose()
	}
}

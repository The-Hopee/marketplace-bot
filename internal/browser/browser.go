// internal/browser/browser.go
package browser

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

type Browser struct {
	browser *rod.Browser
	mu      sync.Mutex
}

var (
	instance *Browser
	once     sync.Once
)

func GetBrowser() *Browser {
	once.Do(func() {
		instance = &Browser{}
		instance.init()
	})
	return instance
}

func (b *Browser) init() {
	path, exists := launcher.LookPath()
	if !exists {
		paths := []string{
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/usr/bin/google-chrome",
		}
		for _, p := range paths {
			path = p
			break
		}
	}

	log.Printf("[Browser] Using Chrome at: %s", path)

	u := launcher.New().
		Bin(path).
		Headless(true).
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-dev-shm-usage").
		Set("disable-gpu").
		Set("no-sandbox").
		Set("disable-setuid-sandbox").
		Set("window-size", "1920,1080").
		Set("lang", "ru-RU,ru").
		Delete("enable-automation").
		MustLaunch()

	b.browser = rod.New().ControlURL(u).MustConnect()
	log.Println("[Browser] Chrome started successfully")
}

func (b *Browser) GetPage(ctx context.Context, url string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Stealth страница
	page, err := stealth.Page(b.browser)
	if err != nil {
		return "", err
	}
	defer page.MustClose()

	// User-Agent
	page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{
		UserAgent:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
		AcceptLanguage: "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7",
		Platform:       "Win32",
	})

	// Размер окна
	page.MustSetViewport(1920, 1080, 1, false)

	// Переходим
	log.Printf("[Browser] Navigating to: %s", url)

	err = rod.Try(func() {
		page.Timeout(60 * time.Second).MustNavigate(url)
	})
	if err != nil {
		return "", err
	}

	// Ждём начальную загрузку
	time.Sleep(3 * time.Second)

	// Проверяем challenge
	html, _ := page.HTML()
	if containsChallenge(html) {
		log.Printf("[Browser] Challenge detected, waiting longer...")
		time.Sleep(8 * time.Second)

		// Пробуем ещё раз получить HTML после ожидания
		html, _ = page.HTML()
		if containsChallenge(html) {
			log.Printf("[Browser] Still challenge, waiting more...")
			time.Sleep(5 * time.Second)
		}
	}

	// Ждём загрузки
	rod.Try(func() {
		page.Timeout(10 * time.Second).MustWaitLoad()
	})

	time.Sleep(3 * time.Second)

	// Скроллим безопасно (через Try)
	rod.Try(func() {
		page.Eval(`window.scrollBy(0, 500)`)
	})
	time.Sleep(1 * time.Second)

	rod.Try(func() {
		page.Eval(`window.scrollBy(0, 500)`)
	})
	time.Sleep(2 * time.Second)

	// Получаем финальный HTML
	html, err = page.HTML()
	if err != nil {
		return "", err
	}

	log.Printf("[Browser] Final page size: %d bytes", len(html))

	return html, nil
}

func containsChallenge(html string) bool {
	challenges := []string{
		"Проверяем браузер",
		"Почти готово",
		"Доступ ограничен",
		"challenge",
		"captcha",
		"не робот",
	}

	htmlLower := strings.ToLower(html)
	for _, c := range challenges {
		if strings.Contains(htmlLower, strings.ToLower(c)) {
			return true
		}
	}
	return false
}

func (b *Browser) Close() {
	if b.browser != nil {
		b.browser.MustClose()
	}
}

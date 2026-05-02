// internal/analysis/ai_agent.go
package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"marketplace-bot/internal/marketplace"

	"github.com/sashabaranov/go-openai"
)

type AIAgent struct {
	client *openai.Client
}

func NewAIAgent(apiKey string, baseURL string) *AIAgent {
	config := openai.DefaultConfig(apiKey)
	// Используем ProxyAPI URL для маршрутизации на Gemini
	if baseURL != "" {
		config.BaseURL = baseURL
	}

	return &AIAgent{
		client: openai.NewClientWithConfig(config),
	}
}

// Analyze PRO-пользователей
func (a *AIAgent) Analyze(ctx context.Context, result *marketplace.AggregatedResult) (string, error) {
	if result.TotalCount == 0 {
		return "😔 К сожалению, я не смог найти товары по вашему запросу.", nil
	}

	type CompactProduct struct {
		Name        string  `json:"name"`
		Price       float64 `json:"price"`
		Discount    int     `json:"discount,omitempty"`
		URL         string  `json:"url"`
		Marketplace string  `json:"marketplace"`
		Condition   string  `json:"condition,omitempty"`
	}

	var compactData []CompactProduct

	for mpName, products := range result.Results {
		limit := min(3, len(products))
		for i := 0; i < limit; i++ {
			p := products[i]

			cp := CompactProduct{
				Name:        truncateUTF8(p.Name, 100),
				Price:       p.Price,
				Discount:    p.Discount,
				URL:         p.URL,
				Marketplace: mpName,
				Condition:   p.Condition,
			}
			compactData = append(compactData, cp)
		}
	}

	if len(compactData) == 0 {
		return "😔 Нашел товары, но не смог подготовить данные для анализа.", nil
	}

	jsonData, err := json.Marshal(compactData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal data for AI: %w", err)
	}

	systemPrompt := `Ты — профессиональный шопинг-ассистент и аналитик маркетплейсов с острым чувством юмора.
Твоя задача: проанализировать предоставленный JSON с товарами (Wildberries, Ozon, Avito).
СТИЛЬ ОТВЕТА:
- Будь эмоциональным и энтузиастичным! 🔥
- Используй яркие, запоминающиеся фразы
- Добавляй уместный юмор и эмпатию
- Будь честным о достоинствах и недостатках

ПРАВИЛА АНАЛИЗА:
1. Если пользователь ищет дорогую технику (смартфон, ноутбук), а в списке есть дешевые чехлы, стекла, коробки или аксессуары — ИГНОРИРУЙ ИХ! Анализируй только саму технику.
2. Если у товара Price равна 0, это значит цена не указана в предпросмотре. Не выкидывай товар! Просто напиши "Цена по запросу".
3. Выбери САМЫЙ ЛУЧШИЙ товар среди конкретного маркетплейса.
4. Выбери АБСОЛЮТНО ЛУЧШИЙ товар среди ВСЕХ маркетплейсов и обоснуй выбор.
5. Если лучший товар с Avito, ОЩУТИМО сделай акцент на его состоянии ("Новое" или "Б/У").
6. Сравни товары по цене, скидке, репутации маркетплейса - но главное - помогай пользователю принять умное решение.

ФОРМАТ ОТВЕТА:
- Структурированный и читаемый
- Используй эмодзи (📦, 💰, ⚠️, 🏆, 😍, 🎯) но не переборщи
- В конце приложи ссылки на победителей
- Не пиши код или сырой JSON в ответе
- Объем: 500-800 символов - информативно, но лаконично`

	req := openai.ChatCompletionRequest{
		Model: "google/gemini-2.0-flash", // Gemini 2.0 Flash через ProxyAPI
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: fmt.Sprintf("Запрос пользователя: '%s'. Данные товаров:\n%s", result.Query, string(jsonData)),
			},
		},
		Temperature: 0.7,
	}

	log.Printf("[AIAgent] Sending request to Gemini 2.0 Flash (via ProxyAPI) for query: %s", result.Query)

	resp, err := a.client.CreateChatCompletion(ctx, req)
	if err != nil {
		log.Printf("[AIAgent] Error calling ProxyAPI/Gemini: %v", err)
		return "", fmt.Errorf("ошибка при анализе ИИ: %w", err)
	}

	return resp.Choices[0].Message.Content, nil
}

// Вспомогательные функции
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func truncateUTF8(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

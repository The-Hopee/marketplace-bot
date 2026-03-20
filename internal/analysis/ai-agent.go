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
	// Если используешь российский прокси (например, ProxyAPI), переопределяем BaseURL
	if baseURL != "" && baseURL != "https://api.openai.com/v1" {
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

			// МЫ БОЛЬШЕ НЕ УДАЛЯЕМ ТОВАРЫ С НУЛЕВОЙ ЦЕНОЙ!
			// ИИ сам разберется.

			cp := CompactProduct{
				Name:        truncateUTF8(p.Name, 100), // Увеличили длину для большей информативности
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

	systemPrompt := `Ты — профессиональный шопинг-ассистент и аналитик маркетплейсов.
Твоя задача: проанализировать предоставленный JSON с товарами (Wildberries, Ozon, Avito).
ВАЖНЫЕ ПРАВИЛА:
1. Если пользователь ищет дорогую технику (смартфон, ноутбук), а в списке есть дешевые чехлы, стекла, коробки или аксессуары — ИГНОРИРУЙ ИХ! Анализируй только саму технику.
2. Если у товара Price равна 0, это значит цена не указана в предпросмотре. Не выкидывай товар! Просто напиши "Цена по запросу на сайте".
3. Выбери САМЫЙ ЛУЧШИЙ товар среди конкретного маркетплейса.
4. Выбери АБСОЛЮТНО ЛУЧШИЙ товар среди ВСЕХ маркетплейсов и обоснуй выбор.
5. Если лучший товар с Avito, ОЩУТИМО сделай акцент на его состоянии ("Новое" или "Б/У").
Отвечай структурированно, используй эмодзи (📦, 💰, ⚠️, 🏆). В конце приложи ссылки на победителей. Не пиши код или сырой JSON в ответе.`

	req := openai.ChatCompletionRequest{
		Model: "gpt-4o-mini", // Дешево и круто
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

	log.Printf("[AIAgent] Sending request to OpenAI for query: %s", result.Query)

	resp, err := a.client.CreateChatCompletion(ctx, req)
	if err != nil {
		log.Printf("[AIAgent] Error calling OpenAI: %v", err)
		return "", fmt.Errorf("ошибка при анализе ИИ: %w", err)
	}

	return resp.Choices[0].Message.Content, nil
}

// Вспомогательные функции (аналогичные тем, что в handler.go)
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

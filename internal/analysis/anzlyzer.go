// internal/analysis/analyzer.go
package analysis

import (
	"fmt"
	"sort"

	"marketplace-bot/internal/marketplace"
)

type AnalysisResult struct {
	Query          string          `json:"query"`
	TotalProducts  int             `json:"total_products"`
	BestByPrice    *ScoredProduct  `json:"best_by_price"`
	BestByDiscount *ScoredProduct  `json:"best_by_discount"`
	BestOverall    *ScoredProduct  `json:"best_overall"`
	PriceStats     PriceStats      `json:"price_stats"`
	TopProducts    []ScoredProduct `json:"top_products"`
	Recommendation string          `json:"recommendation"`
}

type ScoredProduct struct {
	marketplace.Product
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

type PriceStats struct {
	MinPrice     float64 `json:"min_price"`
	MaxPrice     float64 `json:"max_price"`
	AvgPrice     float64 `json:"avg_price"`
	AvgDiscount  float64 `json:"avg_discount"`
	TotalSavings float64 `json:"total_savings"`
}

type Analyzer struct{}

func NewAnalyzer() *Analyzer {
	return &Analyzer{}
}

// Анализ результатов поиска
func (a *Analyzer) Analyze(results *marketplace.AggregatedResult) *AnalysisResult {
	analysis := &AnalysisResult{
		Query: results.Query,
	}

	// Собираем все товары
	var allProducts []marketplace.Product
	for _, products := range results.Results {
		allProducts = append(allProducts, products...)
	}

	if len(allProducts) == 0 {
		analysis.Recommendation = "Товары не найдены"
		return analysis
	}

	analysis.TotalProducts = len(allProducts)

	// Считаем статистику цен
	analysis.PriceStats = a.calculatePriceStats(allProducts)

	// Скорим товары
	scoredProducts := a.scoreProducts(allProducts, analysis.PriceStats)

	// Сортируем по скору
	sort.Slice(scoredProducts, func(i, j int) bool {
		return scoredProducts[i].Score > scoredProducts[j].Score
	})

	// Лучший по общему скору
	if len(scoredProducts) > 0 {
		analysis.BestOverall = &scoredProducts[0]
	}

	// Лучший по цене
	analysis.BestByPrice = a.findBestByPrice(scoredProducts)

	// Лучший по скидке
	analysis.BestByDiscount = a.findBestByDiscount(scoredProducts)

	// Топ-5 товаров
	topCount := min(5, len(scoredProducts))
	analysis.TopProducts = scoredProducts[:topCount]

	// Генерируем рекомендацию
	analysis.Recommendation = a.generateRecommendation(analysis)

	return analysis
}

func (a *Analyzer) calculatePriceStats(products []marketplace.Product) PriceStats {
	if len(products) == 0 {
		return PriceStats{}
	}

	stats := PriceStats{
		MinPrice: products[0].Price,
		MaxPrice: products[0].Price,
	}

	var totalPrice, totalDiscount, totalSavings float64
	discountCount := 0

	for _, p := range products {
		if p.Price > 0 {
			if p.Price < stats.MinPrice {
				stats.MinPrice = p.Price
			}
			if p.Price > stats.MaxPrice {
				stats.MaxPrice = p.Price
			}
			totalPrice += p.Price
		}

		if p.Discount > 0 {
			totalDiscount += float64(p.Discount)
			discountCount++
		}

		if p.OldPrice > p.Price {
			totalSavings += p.OldPrice - p.Price
		}
	}

	stats.AvgPrice = totalPrice / float64(len(products))
	if discountCount > 0 {
		stats.AvgDiscount = totalDiscount / float64(discountCount)
	}
	stats.TotalSavings = totalSavings

	return stats
}

func (a *Analyzer) scoreProducts(products []marketplace.Product, stats PriceStats) []ScoredProduct {
	var scored []ScoredProduct

	priceRange := stats.MaxPrice - stats.MinPrice
	if priceRange == 0 {
		priceRange = 1
	}

	for _, p := range products {
		sp := ScoredProduct{Product: p}

		// Скор по цене (0-40 баллов) - чем ниже цена, тем лучше
		if p.Price > 0 {
			priceScore := (1 - (p.Price-stats.MinPrice)/priceRange) * 40
			sp.Score += priceScore
		}

		// Скор по скидке (0-30 баллов)
		discountScore := float64(p.Discount) / 100 * 30
		sp.Score += discountScore

		// Скор по рейтингу (0-20 баллов)
		if p.Rating > 0 {
			ratingScore := (p.Rating / 5) * 20
			sp.Score += ratingScore
		}
		// Скор по количеству отзывов (0-10 баллов)
		if p.ReviewCount > 0 {
			reviewScore := min(float64(p.ReviewCount)/1000*10, 10)
			sp.Score += reviewScore
		}

		// Определяем причину рекомендации
		sp.Reason = a.determineReason(p, stats)

		scored = append(scored, sp)
	}

	return scored
}

func (a *Analyzer) determineReason(p marketplace.Product, stats PriceStats) string {
	reasons := []string{}

	if p.Price <= stats.MinPrice*1.1 {
		reasons = append(reasons, "низкая цена")
	}
	if p.Discount >= 30 {
		reasons = append(reasons, fmt.Sprintf("скидка %d%%", p.Discount))
	}
	if p.Rating >= 4.5 {
		reasons = append(reasons, fmt.Sprintf("рейтинг %.1f", p.Rating))
	}
	if p.ReviewCount >= 100 {
		reasons = append(reasons, fmt.Sprintf("%d+ отзывов", p.ReviewCount))
	}

	if len(reasons) == 0 {
		return "хороший вариант"
	}

	return fmt.Sprintf("%s", reasons[0])
}

func (a *Analyzer) findBestByPrice(products []ScoredProduct) *ScoredProduct {
	if len(products) == 0 {
		return nil
	}

	best := &products[0]
	for i := range products {
		if products[i].Price > 0 && products[i].Price < best.Price {
			best = &products[i]
		}
	}
	best.Reason = "самая низкая цена"
	return best
}

func (a *Analyzer) findBestByDiscount(products []ScoredProduct) *ScoredProduct {
	if len(products) == 0 {
		return nil
	}

	best := &products[0]
	for i := range products {
		if products[i].Discount > best.Discount {
			best = &products[i]
		}
	}
	if best.Discount > 0 {
		best.Reason = fmt.Sprintf("максимальная скидка %d%%", best.Discount)
		return best
	}
	return nil
}

func (a *Analyzer) generateRecommendation(analysis *AnalysisResult) string {
	if analysis.BestOverall == nil {
		return "Недостаточно данных для рекомендации"
	}

	rec := fmt.Sprintf("🏆 Лучший выбор: %s за %.0f руб.",
		truncate(analysis.BestOverall.Name, 40),
		analysis.BestOverall.Price)

	if analysis.BestOverall.Discount > 0 {
		rec += fmt.Sprintf(" (скидка %d%%)", analysis.BestOverall.Discount)
	}

	return rec
}

func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

package ratelimiter

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// PerMarketplaceLimiter управляет rate limiting для каждого маркетплейса
type PerMarketplaceLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
}

func New() *PerMarketplaceLimiter {
	return &PerMarketplaceLimiter{
		limiters: map[string]*rate.Limiter{
			"wildberries": rate.NewLimiter(rate.Every(2*time.Second), 1), // 1 запрос в 2 сек
			"ozon":        rate.NewLimiter(rate.Every(1500*time.Millisecond), 1),
			"yandex":      rate.NewLimiter(rate.Every(1*time.Second), 2),
		},
	}
}

func (p *PerMarketplaceLimiter) Wait(ctx context.Context, marketplace string) error {
	p.mu.RLock()
	limiter, exists := p.limiters[marketplace]
	p.mu.RUnlock()

	if !exists {
		return nil
	}

	return limiter.Wait(ctx)
}

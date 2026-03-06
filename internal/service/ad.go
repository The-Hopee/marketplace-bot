package service

import (
	"context"
	"math/rand"
	"sync"

	"marketplace-bot/internal/database"
)

type AdService struct {
	repo  *database.Repository
	cache []database.Ad
	mu    sync.RWMutex
}

func NewAdService(repo *database.Repository) *AdService {
	svc := &AdService{repo: repo}
	svc.RefreshCache(context.Background())
	return svc
}

func (s *AdService) RefreshCache(ctx context.Context) {
	ads, err := s.repo.GetActiveAds(ctx)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.cache = ads
	s.mu.Unlock()
}

func (s *AdService) GetRandomAd() *database.Ad {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.cache) == 0 {
		return nil
	}
	ad := s.cache[rand.Intn(len(s.cache))]
	return &ad
}

// internal/cache/redis.go
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisCache(redisURL string, ttl time.Duration) (*RedisCache, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	log.Println("[Cache] Redis connected")

	return &RedisCache{
		client: client,
		ttl:    ttl,
	}, nil
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}

// Генерация ключа
func (c *RedisCache) searchKey(query string) string {
	normalized := strings.ToLower(strings.TrimSpace(query))
	normalized = strings.Join(strings.Fields(normalized), " ")
	return fmt.Sprintf("search:%s", normalized)
}

// Сохраняем результаты поиска
func (c *RedisCache) SetSearchResults(ctx context.Context, query string, data interface{}) error {
	key := c.searchKey(query)

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, key, jsonData, c.ttl).Err()
}

// Получаем результаты поиска из кэша
func (c *RedisCache) GetSearchResults(ctx context.Context, query string, dest interface{}) (bool, error) {
	key := c.searchKey(query)

	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil // Кэш пуст
	}
	if err != nil {
		return false, err
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return false, err
	}

	return true, nil
}

// Счётчик популярных запросов
func (c *RedisCache) IncrementSearchCount(ctx context.Context, query string) error {
	normalized := strings.ToLower(strings.TrimSpace(query))
	return c.client.ZIncrBy(ctx, "popular_searches", 1, normalized).Err()
}

// Получить популярные запросы
func (c *RedisCache) GetPopularSearches(ctx context.Context, limit int) ([]string, error) {
	results, err := c.client.ZRevRange(ctx, "popular_searches", 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}
	return results, nil
}

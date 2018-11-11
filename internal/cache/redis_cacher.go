package cache

import (
	"time"
	"github.com/go-redis/redis"
	"github.com/kyokan/chaind/pkg/config"
)

type RedisCacher struct {
	client *redis.Client
}

func NewRedisCacher(cfg *config.RedisConfig) *RedisCacher {
	return &RedisCacher{
		client: redis.NewClient(&redis.Options{
			Addr:     cfg.URL,
			Password: cfg.Password,
			DB:       cfg.DB,
		}),
	}
}

func (r *RedisCacher) Start() error {
	_, err := r.client.Ping().Result()
	return err
}

func (r *RedisCacher) Stop() error {
	return r.client.Close()
}

func (r *RedisCacher) Get(key string) ([]byte, error) {
	res, err := r.client.Get(key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return []byte(res), nil
}

func (r *RedisCacher) Set(key string, value []byte) error {
	return r.client.Set(key, value, 0).Err()
}

func (r *RedisCacher) SetEx(key string, value []byte, expiration time.Duration) error {
	return r.client.Set(key, value, expiration).Err()
}

func (r *RedisCacher) Has(key string) (bool, error) {
	res, err := r.client.Exists(key).Result()
	if err != nil {
		return false, err
	}

	return res == 1, nil
}

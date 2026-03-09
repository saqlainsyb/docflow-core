package db

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
	"github.com/saqlainsyb/docflow-core/internal/config"
)

func ConnectRedis(cfg *config.Config) *redis.Client {
	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("invalid redis URL: %v", err)
	}

	client := redis.NewClient(opts)

	if err := client.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("unable to reach redis: %v", err)
	}

	log.Println("redis connected")
	return client
}
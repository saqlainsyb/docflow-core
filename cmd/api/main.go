package main

import (
	"log"

	"github.com/saqlainsyb/docflow-core/internal/config"
	"github.com/saqlainsyb/docflow-core/internal/db"
)

func main() {
	cfg := config.Load()

	log.Printf("starting docflow in %s mode on port %s", cfg.AppEnv, cfg.AppPort)

	dbPool := db.Connect(cfg)
	defer dbPool.Close()

	redisClient := db.ConnectRedis(cfg)
	defer redisClient.Close()

	log.Println("all systems ready")
}
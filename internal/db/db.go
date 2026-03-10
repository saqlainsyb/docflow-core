package db

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/saqlainsyb/docflow-core/internal/config"
)

func Connect(cfg *config.Config) *pgxpool.Pool {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("unable to parse database URL: %v", err)
	}

	poolCfg.MaxConns = 25
	poolCfg.MinConns = 2
	poolCfg.MaxConnLifetime = 1 * time.Hour
	poolCfg.MaxConnIdleTime = 30 * time.Minute

	// force all timestamps returned from postgres to be UTC
	poolCfg.ConnConfig.RuntimeParams["timezone"] = "UTC"

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		log.Fatalf("unable to create database pool: %v", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("unable to reach database: %v", err)
	}

	log.Println("database connected")
	return pool
}
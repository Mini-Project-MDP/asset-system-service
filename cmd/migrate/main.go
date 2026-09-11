package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/config"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/database"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fmt.Println("Connecting to Supabase PostgreSQL database...")
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()

	fmt.Println("Connected! Running database schema migration and seeding...")
	if err := database.MigrateAndSeed(ctx, db); err != nil {
		return fmt.Errorf("migrate and seed: %w", err)
	}

	fmt.Println("Database migration and seeding completed successfully!")
	return nil
}

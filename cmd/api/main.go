// @title Asset System Service API
// @version 1.0
// @description Asset Management System Backend Microservice built with Go Fiber.
// @host
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type 'Bearer ' followed by your JWT token.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/delivery/http"
	"github.com/Mini-Project-MDP/asset-system-service/internal/delivery/http/handler"
	"github.com/Mini-Project-MDP/asset-system-service/internal/repository"
	"github.com/Mini-Project-MDP/asset-system-service/internal/service"
	"github.com/Mini-Project-MDP/asset-system-service/pkg/config"
	"github.com/Mini-Project-MDP/asset-system-service/pkg/database"
	"github.com/Mini-Project-MDP/asset-system-service/pkg/jwt"
	"github.com/gofiber/fiber/v3"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	applicationConfig, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	databaseContext, cancelDatabaseContext := context.WithTimeout(
		context.Background(),
		applicationConfig.DatabasePingTimeout,
	)
	databaseConnection, err := database.OpenTurso(
		databaseContext,
		applicationConfig.DatabaseURL,
		applicationConfig.DatabaseAuthToken,
	)
	cancelDatabaseContext()
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer closeDatabase(databaseConnection)

	// Run Database Schema Migrations and Seeding
	if err := database.MigrateAndSeed(context.Background(), databaseConnection); err != nil {
		log.Printf("Warning: Database migration & seed: %v", err)
	}

	tokenManager := jwt.NewTokenManager(applicationConfig.JWTSecret, applicationConfig.JWTExpiryDuration)

	// Composition Root (Dependency Injection)
	authRepo := repository.NewAuthRepository(databaseConnection)
	authService := service.NewAuthService(authRepo, tokenManager)
	authHandler := handler.NewAuthHandler(authService)
	userHandler := handler.NewUserHandler(authService)

	fiberApp := http.NewRouter(http.Dependencies{
		Database:            databaseConnection,
		DatabasePingTimeout: applicationConfig.DatabasePingTimeout,
		TokenManager:        tokenManager,
		Handlers: http.Handlers{
			Auth: authHandler,
			User: userHandler,
		},
	})

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- fiberApp.Listen(applicationConfig.Address(), fiber.ListenConfig{
			DisableStartupMessage: true,
		})
	}()

	log.Printf(
		"asset-system-service started on %s in %s mode; Turso connection ready",
		applicationConfig.Address(),
		applicationConfig.AppEnvironment,
	)

	shutdownSignal, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	select {
	case err := <-serverErrors:
		return fmt.Errorf("serve HTTP: %w", err)
	case <-shutdownSignal.Done():
		log.Print("shutdown signal received")
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()

	if err := fiberApp.ShutdownWithContext(shutdownContext); err != nil {
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}

	return nil
}

func closeDatabase(databaseConnection *sql.DB) {
	if err := databaseConnection.Close(); err != nil {
		log.Printf("close database: %v", err)
	}
}

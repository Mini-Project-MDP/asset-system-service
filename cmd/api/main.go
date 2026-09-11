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

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/client/approvalengine"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/config"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/database"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/delivery/http"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/delivery/http/handler"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/jwt"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/repository"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/service"
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
	databaseConnection, err := database.Open(
		databaseContext,
		applicationConfig.DatabaseURL,
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

	approvalEngineClient := approvalengine.New(applicationConfig.ApprovalEngineBaseURL, applicationConfig.ApprovalEngineAPIKey, nil)
	requestRepo := repository.NewRequestRepository(databaseConnection)
	requestService := service.NewRequestService(requestRepo, approvalEngineClient)

	fiberApp := http.NewRouter(http.Dependencies{
		Database:             databaseConnection,
		DatabasePingTimeout:  applicationConfig.DatabasePingTimeout,
		TokenManager:         tokenManager,
		ApprovalEngineAPIKey: applicationConfig.ApprovalEngineAPIKey,
		Handlers: http.Handlers{
			Auth:    authHandler,
			User:    userHandler,
			Request: handler.NewRequestHandler(requestService),
			Catalog: handler.NewCatalogHandler(databaseConnection),
			Webhook: handler.NewWebhookHandler(requestService),
		},
	})

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- fiberApp.Listen(applicationConfig.Address(), fiber.ListenConfig{
			DisableStartupMessage: true,
		})
	}()

	log.Printf(
		"asset-system-service started on %s in %s mode; Supabase PostgreSQL connection ready",
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

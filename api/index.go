package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/config"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/database"
	appHttp "github.com/Mini-Project-MDP/asset-system-service/internal/pkg/delivery/http"
	appHandler "github.com/Mini-Project-MDP/asset-system-service/internal/pkg/delivery/http/handler"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/jwt"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/repository"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/service"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
)

var (
	mu          sync.Mutex
	initialized bool
	httpHandler http.HandlerFunc
	initErr     error
	db          *sql.DB
)

func initialize() error {
	applicationConfig, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	databaseContext, cancelDatabaseContext := context.WithTimeout(
		context.Background(),
		applicationConfig.DatabasePingTimeout,
	)
	defer cancelDatabaseContext()

	databaseConnection, err := database.OpenTurso(
		databaseContext,
		applicationConfig.DatabaseURL,
		applicationConfig.DatabaseAuthToken,
	)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	db = databaseConnection

	tokenManager := jwt.NewTokenManager(applicationConfig.JWTSecret, applicationConfig.JWTExpiryDuration)
	authRepo := repository.NewAuthRepository(db)
	authService := service.NewAuthService(authRepo, tokenManager)
	authHandlerInstance := appHandler.NewAuthHandler(authService)
	userHandlerInstance := appHandler.NewUserHandler(authService)

	fiberApp := appHttp.NewRouter(appHttp.Dependencies{
		Database:            db,
		DatabasePingTimeout: applicationConfig.DatabasePingTimeout,
		TokenManager:        tokenManager,
		Handlers: appHttp.Handlers{
			Auth:    authHandlerInstance,
			User:    userHandlerInstance,
			Request: appHandler.NewRequestHandler(databaseConnection),
			Catalog: appHandler.NewCatalogHandler(databaseConnection),
		},
	})

	httpHandler = adaptor.FiberApp(fiberApp)
	return nil
}

// Handler is the serverless entrypoint for Vercel deployments.
func Handler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	if !initialized {
		initErr = initialize()
		if initErr == nil {
			initialized = true
		}
	}
	err := initErr
	h := httpHandler
	mu.Unlock()

	if err != nil {
		log.Printf("Vercel handler init error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"success":false,"error":"Initialization error: %v"}`+"\n", err)
		return
	}

	h(w, r)
}

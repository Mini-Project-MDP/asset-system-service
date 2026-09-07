package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"sync"

	appHttp "github.com/Mini-Project-MDP/asset-system-service/pkg/delivery/http"
	appHandler "github.com/Mini-Project-MDP/asset-system-service/pkg/delivery/http/handler"
	"github.com/Mini-Project-MDP/asset-system-service/pkg/repository"
	"github.com/Mini-Project-MDP/asset-system-service/pkg/service"
	"github.com/Mini-Project-MDP/asset-system-service/pkg/config"
	"github.com/Mini-Project-MDP/asset-system-service/pkg/database"
	"github.com/Mini-Project-MDP/asset-system-service/pkg/jwt"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
)

var (
	once        sync.Once
	httpHandler http.HandlerFunc
	initErr     error
	db          *sql.DB
)

func initialize() {
	applicationConfig, err := config.Load()
	if err != nil {
		initErr = fmt.Errorf("load config: %w", err)
		return
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
		initErr = fmt.Errorf("connect database: %w", err)
		return
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
			Auth: authHandlerInstance,
			User: userHandlerInstance,
		},
	})

	httpHandler = adaptor.FiberApp(fiberApp)
}

// Handler is the serverless entrypoint for Vercel deployments.
func Handler(w http.ResponseWriter, r *http.Request) {
	once.Do(initialize)

	if initErr != nil {
		log.Printf("Vercel handler init error: %v", initErr)
		http.Error(w, fmt.Sprintf("Initialization error: %v", initErr), http.StatusInternalServerError)
		return
	}

	httpHandler(w, r)
}

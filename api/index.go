package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/client/approvalengine"
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

	databaseConnection, err := database.Open(
		databaseContext,
		applicationConfig.DatabaseURL,
	)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	db = databaseConnection

	tokenManager := jwt.NewTokenManager(applicationConfig.JWTSecret, applicationConfig.JWTExpiryDuration)
	authRepo := repository.NewAuthRepository(db)
	authService := service.NewAuthService(authRepo, tokenManager)
	permissionResolver := service.NewPermissionResolver(authRepo, time.Minute)
	userService := service.NewUserService(repository.NewUserAdminRepository(databaseConnection))
	authHandlerInstance := appHandler.NewAuthHandler(authService)
	userHandlerInstance := appHandler.NewUserHandler(authService, userService)

	approvalEngineClient := approvalengine.New(applicationConfig.ApprovalEngineBaseURL, applicationConfig.ApprovalEngineAPIKey, nil)
	requestRepo := repository.NewRequestRepository(databaseConnection)
	requestService := service.NewRequestService(requestRepo, approvalEngineClient)
	masterDataService := service.NewMasterDataService(repository.NewMasterDataRepository(databaseConnection))
	phoneCatalogService := service.NewPhoneCatalogService(repository.NewPhoneCatalogRepository(databaseConnection))
	imeiService := service.NewImeiService(repository.NewImeiReferenceRepository(databaseConnection))
	dashboardService := service.NewDashboardService(repository.NewDashboardRepository(databaseConnection), time.Now)

	fiberApp := appHttp.NewRouter(appHttp.Dependencies{
		Database:             db,
		DatabasePingTimeout:  applicationConfig.DatabasePingTimeout,
		TokenManager:         tokenManager,
		AllowedOrigins:       applicationConfig.AllowedOrigins,
		ApprovalEngineAPIKey: applicationConfig.ApprovalEngineAPIKey,
		Permissions:          permissionResolver,
		Handlers: appHttp.Handlers{
			Auth:         authHandlerInstance,
			User:         userHandlerInstance,
			Request:      appHandler.NewRequestHandler(requestService),
			Dashboard:    appHandler.NewDashboardHandler(dashboardService),
			MasterData:   appHandler.NewMasterDataHandler(masterDataService),
			PhoneCatalog: appHandler.NewPhoneCatalogHandler(phoneCatalogService),
			Imei:         appHandler.NewImeiHandler(imeiService),
			Webhook:      appHandler.NewWebhookHandler(requestService),
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

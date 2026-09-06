package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/Mini-Project-MDP/asset-system-service/pkg/app"
	"github.com/Mini-Project-MDP/asset-system-service/pkg/config"
	"github.com/Mini-Project-MDP/asset-system-service/pkg/infrastructure/database"
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

	fiberApp := app.NewRouter(app.Dependencies{
		Database:            db,
		DatabasePingTimeout: applicationConfig.DatabasePingTimeout,
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

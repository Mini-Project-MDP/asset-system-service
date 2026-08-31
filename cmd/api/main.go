package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/app"
	"github.com/Mini-Project-MDP/asset-system-service/internal/config"
	"github.com/Mini-Project-MDP/asset-system-service/internal/infrastructure/database"
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

	server := &http.Server{
		Addr: applicationConfig.Address(),
		Handler: app.NewRouter(app.Dependencies{
			Database:            databaseConnection,
			DatabasePingTimeout: applicationConfig.DatabasePingTimeout,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	listener, err := net.Listen("tcp", applicationConfig.Address())
	if err != nil {
		return fmt.Errorf("listen on %s: %w", applicationConfig.Address(), err)
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Serve(listener)
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
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
	case <-shutdownSignal.Done():
		log.Print("shutdown signal received")
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownContext); err != nil {
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}

	return nil
}

func closeDatabase(databaseConnection *sql.DB) {
	if err := databaseConnection.Close(); err != nil {
		log.Printf("close database: %v", err)
	}
}

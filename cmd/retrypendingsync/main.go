// Command retrypendingsync re-attempts Approval-Engine-Service registration
// for every asset request stuck at approval_status=PENDING_ENGINE_SYNC (an
// earlier create-time sync failed — see docs/approval-engine-integration-plan.md
// Milestone 5). Meant to be invoked periodically by an external scheduler
// (cron, a CI job, etc.) — the HTTP server does not run this itself.
//
// Usage:
//
//	go run ./cmd/retrypendingsync
//
// Exits non-zero if any retry still failed, so a cron wrapper can alert on
// persistent failures without parsing output.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/client/approvalengine"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/config"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/database"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/repository"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/service"
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()

	engine := approvalengine.New(cfg.ApprovalEngineBaseURL, cfg.ApprovalEngineAPIKey, nil)
	requestService := service.NewRequestService(repository.NewRequestRepository(db), engine)

	retried, failed, err := requestService.RetryPendingApprovalSync(ctx)
	if err != nil {
		return fmt.Errorf("retry pending approval sync: %w", err)
	}

	log.Printf("retry pending approval sync: %d synced, %d still pending", retried, failed)
	if failed > 0 {
		os.Exit(1)
	}
	return nil
}

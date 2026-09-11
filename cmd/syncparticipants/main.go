// Command syncparticipants pushes users + m_employee org-chart data from
// asset-system-service's database into Approval-Engine-Service's participant
// store (POST /api/v1/participants/import), per
// docs/approval-engine-integration-plan.md Milestone 3.
//
// This is the one-off/manual version described in the milestone plan; a
// scheduled or event-triggered version can wrap the same syncOnce logic
// later without changing it.
//
// Usage:
//
//	go run ./cmd/syncparticipants
//
// Reads TURSO_DATABASE_URL/TURSO_AUTH_TOKEN and APPROVAL_ENGINE_BASE_URL/
// APPROVAL_ENGINE_API_KEY from .env (or the process environment).
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/config"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/database"
)

// participant mirrors Approval-Engine-Service's internal/domain.Participant
// JSON shape. Duplicated here (not imported) because the two services are
// separate modules/repos with no shared package.
type participant struct {
	UserID     string  `json:"user_id"`
	Name       string  `json:"name"`
	Email      string  `json:"email,omitempty"`
	Position   string  `json:"position,omitempty"`
	Department string  `json:"department,omitempty"`
	SuperiorID *string `json:"superior_id,omitempty"`
	IsActive   bool    `json:"is_active"`
}

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

	engineBaseURL := requireEnv("APPROVAL_ENGINE_BASE_URL")
	engineAPIKey := requireEnv("APPROVAL_ENGINE_API_KEY")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()

	participants, err := loadParticipants(ctx, db)
	if err != nil {
		return fmt.Errorf("load participants: %w", err)
	}
	if len(participants) == 0 {
		log.Println("no participants found (users JOIN m_employee returned nothing) — nothing to sync")
		return nil
	}

	if err := importParticipants(ctx, engineBaseURL, engineAPIKey, participants); err != nil {
		return fmt.Errorf("import participants: %w", err)
	}

	log.Printf("synced %d participant(s) to Approval-Engine-Service", len(participants))
	return nil
}

// loadParticipants joins users (identity, status) with m_employee (org
// chart: superior_id) and user_roles/roles (primary role code, used as
// Position — see docs/backend-milestones.md Milestone 3 for why department
// data isn't available yet). employee_no is the id sent to the engine (see
// Fase 0 in approval-engine-integration-plan.md: employee_no, not the
// internal UUID, is the identifier shared across systems).
func loadParticipants(ctx context.Context, db *sql.DB) ([]participant, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			u.employee_no,
			u.name,
			u.email,
			u.status,
			COALESCE(r.code, ''),
			me.superior_id,
			COALESCE(me.is_vacant, 'N'),
			COALESCE(me.is_terminate, 'N')
		FROM users u
		JOIN m_employee me ON me.user_id = u.id
		LEFT JOIN user_roles ur ON ur.user_id = u.id AND ur.is_primary = 1
		LEFT JOIN roles r ON r.id = ur.role_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []participant
	for rows.Next() {
		var employeeNo, name, email, status, position, isVacant, isTerminate string
		var superiorNIK sql.NullString
		if err := rows.Scan(&employeeNo, &name, &email, &status, &position, &superiorNIK, &isVacant, &isTerminate); err != nil {
			return nil, err
		}

		p := participant{
			UserID:   employeeNo,
			Name:     name,
			Email:    email,
			Position: position,
			IsActive: status == "ACTIVE" && isVacant != "Y" && isTerminate != "Y",
		}
		if superiorNIK.Valid && superiorNIK.String != "" {
			superior := superiorNIK.String
			p.SuperiorID = &superior
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func importParticipants(ctx context.Context, baseURL, apiKey string, participants []participant) error {
	body, err := json.Marshal(struct {
		Participants []participant `json:"participants"`
	}{Participants: participants})
	if err != nil {
		return fmt.Errorf("marshal participants: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/participants/import", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Note: /participants/import is not behind X-API-Key in the engine's
	// current router (see Approval-Engine-Service/internal/router/router.go)
	// — only POST /requests is. Sent anyway so this keeps working once that
	// gap is closed.
	req.Header.Set("X-API-Key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("call engine: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("engine responded %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is required (set it in .env or the environment)", key)
	}
	return v
}

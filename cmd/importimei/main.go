// Command importimei loads the IMEI/TAC reference table (the data behind the
// Android fulfillment form's IMEI auto-fill) from a CSV file.
//
// The reference data itself, normally the GSMA TAC database, is licensed and not
// part of this repository. Usage:
//
//	go run ./cmd/importimei -file tac.csv            # import (insert or update by TAC)
//	go run ./cmd/importimei -file tac.csv -dry-run   # validate only, touch nothing
//
// CSV columns: tac (8 digits), brand, model, release_year (4 digits). A header
// row and blank lines are optional; see seeds/imei_reference_template.csv.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

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
	file := flag.String("file", "", "CSV file with columns tac,brand,model,release_year (required)")
	dryRun := flag.Bool("dry-run", false, "validate the file and report, without touching the database")
	flag.Parse()
	if *file == "" {
		flag.Usage()
		return fmt.Errorf("-file is required")
	}

	f, err := os.Open(*file)
	if err != nil {
		return fmt.Errorf("open %s: %w", *file, err)
	}
	defer f.Close()

	rows, err := service.ParseImeiReferenceCSV(f)
	if err != nil {
		return err
	}
	fmt.Printf("%s: %d valid rows\n", *file, len(rows))
	if *dryRun {
		fmt.Println("dry run: nothing was written")
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()

	n, err := repository.ImportImeiReference(ctx, db, rows)
	if err != nil {
		return err
	}
	fmt.Printf("imported %d rows into imei_reference\n", n)
	return nil
}

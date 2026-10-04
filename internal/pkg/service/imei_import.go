package service

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

// maxReportedImportErrors caps how many problems one import reports, so a
// wrong file does not produce thousands of lines.
const maxReportedImportErrors = 20

// ParseImeiReferenceCSV reads TAC reference rows from CSV with the columns
// tac, brand, model, release_year. A header row and blank lines are optional.
// Every invalid line is reported with its line number, and nothing is returned
// unless the whole file is valid, so an import is all-or-nothing.
func ParseImeiReferenceCSV(r io.Reader) ([]domain.ImeiReferenceRow, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1 // checked per line, to name the line in the error
	reader.TrimLeadingSpace = true

	var rows []domain.ImeiReferenceRow
	var problems []string
	seen := map[string]int{}
	report := func(line int, format string, args ...any) {
		if len(problems) < maxReportedImportErrors {
			problems = append(problems, fmt.Sprintf("line %d: %s", line, fmt.Sprintf(format, args...)))
		}
	}

	first := true
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line, _ := reader.FieldPos(0)
		if err != nil {
			report(line, "%v", err)
			continue
		}
		if len(record) == 1 && strings.TrimSpace(record[0]) == "" {
			continue // blank line
		}
		if first {
			first = false
			if strings.EqualFold(strings.TrimSpace(record[0]), "tac") {
				continue // header
			}
		}

		if len(record) != 4 {
			report(line, "expected 4 columns (tac, brand, model, release_year), got %d", len(record))
			continue
		}
		row := domain.ImeiReferenceRow{
			TAC:         strings.TrimSpace(record[0]),
			Brand:       strings.TrimSpace(record[1]),
			Model:       strings.TrimSpace(record[2]),
			ReleaseYear: strings.TrimSpace(record[3]),
		}
		switch {
		case len(row.TAC) != tacLength || !isDigits(row.TAC):
			report(line, "tac must be exactly %d digits, got %q", tacLength, row.TAC)
		case row.Brand == "":
			report(line, "brand is required")
		case row.Model == "":
			report(line, "model is required")
		case !isFourDigitYear(row.ReleaseYear):
			report(line, "release_year must be a 4-digit year, got %q", row.ReleaseYear)
		default:
			if firstLine, dup := seen[row.TAC]; dup {
				report(line, "duplicate tac %s (first seen on line %d)", row.TAC, firstLine)
				continue
			}
			seen[row.TAC] = line
			rows = append(rows, row)
		}
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("invalid IMEI reference file:\n  %s", strings.Join(problems, "\n  "))
	}
	if len(rows) == 0 {
		return nil, errors.New("invalid IMEI reference file: no data rows")
	}
	return rows, nil
}

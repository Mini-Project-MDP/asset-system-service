package service

import (
	"strings"
	"testing"
)

func TestParseImeiReferenceCSV(t *testing.T) {
	t.Run("parses rows with a header", func(t *testing.T) {
		rows, err := ParseImeiReferenceCSV(strings.NewReader(
			"tac,brand,model,release_year\n35489201,Samsung,Galaxy Tab,2023\n86492019, Xiaomi ,Redmi Note,2022\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 2 || rows[0].TAC != "35489201" || rows[1].Brand != "Xiaomi" || rows[1].ReleaseYear != "2022" {
			t.Fatalf("rows = %+v", rows)
		}
	})

	t.Run("a header is optional and blank lines are skipped", func(t *testing.T) {
		rows, err := ParseImeiReferenceCSV(strings.NewReader("\n35489201,Samsung,Galaxy Tab,2023\n\n"))
		if err != nil || len(rows) != 1 {
			t.Fatalf("rows = %+v, err = %v", rows, err)
		}
	})

	t.Run("quoted fields may contain commas", func(t *testing.T) {
		rows, err := ParseImeiReferenceCSV(strings.NewReader(`35489201,Samsung,"Galaxy Tab, 10.1",2023` + "\n"))
		if err != nil || len(rows) != 1 || rows[0].Model != "Galaxy Tab, 10.1" {
			t.Fatalf("rows = %+v, err = %v", rows, err)
		}
	})

	t.Run("reports every bad line with its line number", func(t *testing.T) {
		_, err := ParseImeiReferenceCSV(strings.NewReader(strings.Join([]string{
			"tac,brand,model,release_year",
			"35489200,Samsung,A,2023", // line 2: valid
			"1234567,Samsung,A,2023",  // line 3: TAC too short
			"35489202,,A,2023",        // line 4: brand missing
			"35489203,Oppo,,2023",     // line 5: model missing
			"35489204,Oppo,A,23",      // line 6: bad year
			"35489205,Oppo,A",         // line 7: wrong column count
			"35489200,Samsung,B,2023", // line 8: duplicate of the valid line 2
			"35489206,Vivo,V,2021",    // line 9: valid
		}, "\n")))
		if err == nil {
			t.Fatal("expected an error")
		}
		for _, want := range []string{"line 3", "line 4", "line 5", "line 6", "line 7", "line 8"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error should mention %q:\n%v", want, err)
			}
		}
		for _, notWant := range []string{"line 2:", "line 9:"} {
			if strings.Contains(err.Error(), notWant) {
				t.Errorf("valid line must not be reported (%q):\n%v", notWant, err)
			}
		}
	})

	t.Run("an empty file is an error", func(t *testing.T) {
		if _, err := ParseImeiReferenceCSV(strings.NewReader("tac,brand,model,release_year\n")); err == nil {
			t.Fatal("expected an error for a file without data rows")
		}
	})
}

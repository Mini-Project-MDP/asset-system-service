package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

type fakeImeiRepository struct {
	byTAC map[string]domain.ImeiInfo
	asked []string
}

func (f *fakeImeiRepository) LookupTAC(ctx context.Context, tac string) (*domain.ImeiInfo, error) {
	f.asked = append(f.asked, tac)
	if info, ok := f.byTAC[tac]; ok {
		return &info, nil
	}
	return nil, nil
}

func TestImeiLookup(t *testing.T) {
	ctx := context.Background()
	repo := &fakeImeiRepository{byTAC: map[string]domain.ImeiInfo{
		"35489201": {Brand: "Samsung", Model: "Galaxy Tab", ReleaseYear: "2023"},
	}}
	svc := NewImeiService(repo)

	t.Run("a known TAC returns the device", func(t *testing.T) {
		got, err := svc.Lookup(ctx, "354892019283741")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || got.Brand != "Samsung" || got.Model != "Galaxy Tab" || got.ReleaseYear != "2023" {
			t.Fatalf("got %+v, want Samsung Galaxy Tab 2023", got)
		}
	})

	t.Run("only the first 8 digits are looked up", func(t *testing.T) {
		repo.asked = nil
		svc.Lookup(ctx, "354892019283741")
		if len(repo.asked) != 1 || repo.asked[0] != "35489201" {
			t.Fatalf("looked up %v, want [35489201]", repo.asked)
		}
	})

	t.Run("surrounding spaces are ignored; 14 and 16 digit IMEIs work", func(t *testing.T) {
		for _, imei := range []string{" 354892019283741 ", "35489201928374", "3548920192837412"} {
			got, err := svc.Lookup(ctx, imei)
			if err != nil || got == nil {
				t.Errorf("imei %q: got %v, %v; want a match", imei, got, err)
			}
		}
	})

	t.Run("an unknown TAC is not an error", func(t *testing.T) {
		got, err := svc.Lookup(ctx, "990000009283741")
		if err != nil || got != nil {
			t.Fatalf("got %v, %v; want nil, nil", got, err)
		}
	})

	t.Run("an implausible IMEI is rejected without touching the repository", func(t *testing.T) {
		repo.asked = nil
		for _, bad := range []string{"", "1234567", "35489201928374X", "abcdefghijklmno", "35489201928374123"} {
			if _, err := svc.Lookup(ctx, bad); !errors.Is(err, ErrInvalidIMEI) {
				t.Errorf("imei %q: err = %v, want ErrInvalidIMEI", bad, err)
			}
		}
		if len(repo.asked) != 0 {
			t.Fatalf("repository was queried for invalid input: %v", repo.asked)
		}
	})
}

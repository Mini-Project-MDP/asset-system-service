package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

func TestValidateFulfillmentData(t *testing.T) {
	validUnit := `{"imei":"354892019283741","brand":"Samsung","model":"Galaxy Tab","releaseYear":"2023","regYear":"2024"}`

	t.Run("accepts valid data and returns normalized JSON", func(t *testing.T) {
		cases := []struct {
			name, category string
			qty            int
			raw            string
			want           string
		}{
			{"barcode trims codes", "Barcode", 2, `{"codes":[" BC-1 ","BC-2"]}`, `{"codes":["BC-1","BC-2"]}`},
			{"server trims specs", "Server", 4, `{"specs":"  i5, 16GB  "}`, `{"specs":"i5, 16GB"}`},
			{"android", "Android", 1, `{"units":[` + validUnit + `]}`,
				`{"units":[{"imei":"354892019283741","brand":"Samsung","model":"Galaxy Tab","releaseYear":"2023","regYear":"2024"}]}`},
		}
		for _, c := range cases {
			got, err := validateFulfillmentData(c.category, c.qty, c.raw)
			if err != nil {
				t.Errorf("%s: unexpected error: %v", c.name, err)
				continue
			}
			if got != c.want {
				t.Errorf("%s: got %s, want %s", c.name, got, c.want)
			}
		}
	})

	t.Run("rejects invalid data", func(t *testing.T) {
		unit := func(imei string) string {
			return `{"imei":"` + imei + `","brand":"Samsung","model":"Galaxy Tab","releaseYear":"2023","regYear":"2024"}`
		}
		cases := []struct {
			name, category string
			qty            int
			raw, wantMsg   string
		}{
			{"not json", "Barcode", 1, `not json`, "JSON"},
			{"not an object", "Barcode", 1, `["a"]`, "JSON"},
			{"unknown category", "Mobile Printer", 1, `{"codes":["a"]}`, "category"},
			{"barcode too few", "Barcode", 3, `{"codes":["a","b"]}`, "3"},
			{"barcode too many", "Barcode", 1, `{"codes":["a","b"]}`, "1"},
			{"barcode blank code", "Barcode", 2, `{"codes":["a","  "]}`, "2"},
			{"barcode duplicate", "Barcode", 2, `{"codes":["a","a"]}`, "duplicate"},
			{"server empty", "Server", 1, `{"specs":"   "}`, "specs"},
			{"server missing", "Server", 1, `{}`, "specs"},
			{"android wrong count", "Android", 2, `{"units":[` + unit("354892019283741") + `]}`, "2"},
			{"android short imei", "Android", 1, `{"units":[` + unit("12345") + `]}`, "IMEI"},
			{"android letters in imei", "Android", 1, `{"units":[` + unit("35489201928374X") + `]}`, "IMEI"},
			{"android duplicate imei", "Android", 2, `{"units":[` + unit("354892019283741") + `,` + unit("354892019283741") + `]}`, "duplicate"},
			{"android missing brand", "Android", 1, `{"units":[{"imei":"354892019283741","brand":"","model":"X","releaseYear":"2023","regYear":"2024"}]}`, "brand"},
			{"android bad year", "Android", 1, `{"units":[{"imei":"354892019283741","brand":"A","model":"X","releaseYear":"23","regYear":"2024"}]}`, "releaseYear"},
		}
		for _, c := range cases {
			_, err := validateFulfillmentData(c.category, c.qty, c.raw)
			if !errors.Is(err, ErrFulfillmentDataInvalid) {
				t.Errorf("%s: err = %v, want ErrFulfillmentDataInvalid", c.name, err)
				continue
			}
			if !strings.Contains(err.Error(), c.wantMsg) {
				t.Errorf("%s: message %q should mention %q", c.name, err.Error(), c.wantMsg)
			}
		}
	})

	t.Run("android IMEI may be 14, 15 or 16 digits", func(t *testing.T) {
		for _, imei := range []string{"35489201928374", "354892019283741", "3548920192837412"} {
			raw := `{"units":[{"imei":"` + imei + `","brand":"A","model":"X","releaseYear":"2023","regYear":"2024"}]}`
			if _, err := validateFulfillmentData("Android", 1, raw); err != nil {
				t.Errorf("imei %s: unexpected error %v", imei, err)
			}
		}
	})
}

func fulfillmentRepo(status string, step *int, category string, qty int) *fakeRequestRepository {
	return &fakeRequestRepository{items: []domain.AssetRequest{{
		ID: "REQ-1", Category: category, Quantity: qty, Status: status, FulfillmentStep: step,
	}}}
}

func intPtr(i int) *int { return &i }

func TestSaveFulfillmentData(t *testing.T) {
	ctx := context.Background()
	barcodes := `{"codes":["BC-1","BC-2"]}`

	t.Run("unknown request is not found", func(t *testing.T) {
		svc := NewRequestService(fulfillmentRepo(domain.RequestStatusApproved, nil, "Barcode", 2), &fakeApprovalEngineClient{})
		if _, err := svc.SaveFulfillmentData(ctx, "nope", barcodes, "EMP1"); !errors.Is(err, ErrRequestNotFound) {
			t.Fatalf("err = %v, want ErrRequestNotFound", err)
		}
	})

	t.Run("approved request moves to Shipped and stores normalized JSON", func(t *testing.T) {
		repo := fulfillmentRepo(domain.RequestStatusApproved, nil, "Barcode", 2)
		svc := NewRequestService(repo, &fakeApprovalEngineClient{})
		got, err := svc.SaveFulfillmentData(ctx, "REQ-1", `{"codes":[" BC-1 ","BC-2"]}`, "EMP1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Status != domain.RequestStatusFulfillment || got.FulfillmentStep == nil || *got.FulfillmentStep != 1 {
			t.Fatalf("got status %s step %v, want FULFILLMENT at step 1", got.Status, got.FulfillmentStep)
		}
		if got.FulfillmentData == nil || !json.Valid([]byte(*got.FulfillmentData)) || *got.FulfillmentData != `{"codes":["BC-1","BC-2"]}` {
			t.Fatalf("stored data = %v, want normalized JSON", got.FulfillmentData)
		}
		hist := repo.history["REQ-1"]
		if len(hist) != 1 || hist[0].Role != "EMP1" || !strings.Contains(hist[0].Action, "Shipped") {
			t.Fatalf("history = %+v, want one Shipped entry by EMP1", hist)
		}
	})

	t.Run("a request still waiting for approval cannot be fulfilled", func(t *testing.T) {
		repo := fulfillmentRepo(domain.RequestStatusWaitingApproval, nil, "Barcode", 2)
		svc := NewRequestService(repo, &fakeApprovalEngineClient{})
		if _, err := svc.SaveFulfillmentData(ctx, "REQ-1", barcodes, "EMP1"); !errors.Is(err, ErrFulfillmentNotReady) {
			t.Fatalf("err = %v, want ErrFulfillmentNotReady", err)
		}
		if repo.items[0].Status != domain.RequestStatusWaitingApproval || repo.items[0].FulfillmentData != nil {
			t.Fatalf("request was modified: %+v", repo.items[0])
		}
	})

	t.Run("rejected, shipped and completed requests are not ready either", func(t *testing.T) {
		for _, tc := range []struct {
			status string
			step   *int
		}{
			{domain.RequestStatusRejected, nil},
			{domain.RequestStatusFulfillment, intPtr(1)},
			{domain.RequestStatusCompleted, intPtr(3)},
		} {
			svc := NewRequestService(fulfillmentRepo(tc.status, tc.step, "Barcode", 2), &fakeApprovalEngineClient{})
			if _, err := svc.SaveFulfillmentData(ctx, "REQ-1", barcodes, "EMP1"); !errors.Is(err, ErrFulfillmentNotReady) {
				t.Errorf("status %s: err = %v, want ErrFulfillmentNotReady", tc.status, err)
			}
		}
	})

	t.Run("a request already in Processing can be saved", func(t *testing.T) {
		svc := NewRequestService(fulfillmentRepo(domain.RequestStatusFulfillment, intPtr(0), "Barcode", 2), &fakeApprovalEngineClient{})
		if _, err := svc.SaveFulfillmentData(ctx, "REQ-1", barcodes, "EMP1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid data is rejected and nothing changes", func(t *testing.T) {
		repo := fulfillmentRepo(domain.RequestStatusApproved, nil, "Barcode", 2)
		svc := NewRequestService(repo, &fakeApprovalEngineClient{})
		if _, err := svc.SaveFulfillmentData(ctx, "REQ-1", `{"codes":["only-one"]}`, "EMP1"); !errors.Is(err, ErrFulfillmentDataInvalid) {
			t.Fatalf("err = %v, want ErrFulfillmentDataInvalid", err)
		}
		if repo.items[0].Status != domain.RequestStatusApproved || repo.items[0].FulfillmentData != nil || len(repo.history["REQ-1"]) != 0 {
			t.Fatalf("request or history was modified: %+v", repo.items[0])
		}
	})

	t.Run("wrong state is reported before bad data", func(t *testing.T) {
		svc := NewRequestService(fulfillmentRepo(domain.RequestStatusWaitingApproval, nil, "Barcode", 2), &fakeApprovalEngineClient{})
		if _, err := svc.SaveFulfillmentData(ctx, "REQ-1", `garbage`, "EMP1"); !errors.Is(err, ErrFulfillmentNotReady) {
			t.Fatalf("err = %v, want ErrFulfillmentNotReady", err)
		}
	})
}

func TestAdvanceFulfillment(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown request is not found", func(t *testing.T) {
		svc := NewRequestService(fulfillmentRepo(domain.RequestStatusFulfillment, intPtr(1), "Barcode", 1), &fakeApprovalEngineClient{})
		if _, err := svc.AdvanceFulfillment(ctx, "nope", "EMP1"); !errors.Is(err, ErrRequestNotFound) {
			t.Fatalf("err = %v, want ErrRequestNotFound", err)
		}
	})

	t.Run("Shipped goes to Delivered, Delivered goes to Completed", func(t *testing.T) {
		repo := fulfillmentRepo(domain.RequestStatusFulfillment, intPtr(1), "Barcode", 1)
		svc := NewRequestService(repo, &fakeApprovalEngineClient{})

		got, err := svc.AdvanceFulfillment(ctx, "REQ-1", "EMP1")
		if err != nil {
			t.Fatalf("first advance: %v", err)
		}
		if *got.FulfillmentStep != 2 || got.Status != domain.RequestStatusFulfillment {
			t.Fatalf("after first advance: step %d status %s, want 2 FULFILLMENT", *got.FulfillmentStep, got.Status)
		}

		got, err = svc.AdvanceFulfillment(ctx, "REQ-1", "EMP1")
		if err != nil {
			t.Fatalf("second advance: %v", err)
		}
		if *got.FulfillmentStep != 3 || got.Status != domain.RequestStatusCompleted {
			t.Fatalf("after second advance: step %d status %s, want 3 COMPLETED", *got.FulfillmentStep, got.Status)
		}

		hist := repo.history["REQ-1"]
		if len(hist) != 2 || !strings.Contains(hist[0].Action, "Delivered") || !strings.Contains(hist[1].Action, "Completed") {
			t.Fatalf("history = %+v, want Delivered then Completed", hist)
		}
	})

	t.Run("cannot advance before the data was recorded", func(t *testing.T) {
		for _, tc := range []struct {
			status string
			step   *int
		}{
			{domain.RequestStatusApproved, nil},
			{domain.RequestStatusFulfillment, intPtr(0)},
			{domain.RequestStatusWaitingApproval, nil},
		} {
			svc := NewRequestService(fulfillmentRepo(tc.status, tc.step, "Barcode", 1), &fakeApprovalEngineClient{})
			if _, err := svc.AdvanceFulfillment(ctx, "REQ-1", "EMP1"); !errors.Is(err, ErrFulfillmentNotReady) {
				t.Errorf("status %s: err = %v, want ErrFulfillmentNotReady", tc.status, err)
			}
		}
	})

	t.Run("a completed request cannot advance again", func(t *testing.T) {
		svc := NewRequestService(fulfillmentRepo(domain.RequestStatusCompleted, intPtr(3), "Barcode", 1), &fakeApprovalEngineClient{})
		if _, err := svc.AdvanceFulfillment(ctx, "REQ-1", "EMP1"); !errors.Is(err, ErrFulfillmentNotReady) {
			t.Fatalf("err = %v, want ErrFulfillmentNotReady", err)
		}
	})
}

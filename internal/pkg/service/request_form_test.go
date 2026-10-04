package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

type fakeFormSource struct {
	data domain.FormMasterData
	err  error
}

func (f fakeFormSource) FormMasterData(ctx context.Context) (domain.FormMasterData, error) {
	return f.data, f.err
}

func formMasterData() domain.FormMasterData {
	bandung := domain.OutletRef{ID: "o-bdg", Name: "Bandung Kota"}
	depok := domain.OutletRef{ID: "o-dpk", Name: "Depok Tengah"}
	surabaya := domain.OutletRef{ID: "o-sby", Name: "Surabaya Timur"}
	return domain.FormMasterData{
		Distributors: []domain.Distributor{
			{ID: "d-utama", Name: "PT Utama", IsActive: true, Outlets: []domain.OutletRef{bandung, depok}},
			{ID: "d-lain", Name: "PT Lain", IsActive: true, Outlets: []domain.OutletRef{surabaya}},
		},
		Outlets: []domain.Outlet{
			{ID: "o-bdg", Name: "Bandung Kota"}, {ID: "o-dpk", Name: "Depok Tengah"},
			{ID: "o-sby", Name: "Surabaya Timur"}, {ID: "o-mdn", Name: "Medan Kota"}, // Medan is mapped to no distributor
		},
		SalesDivisions: []string{"M1 BIS", "M3"},
	}
}

func validInput() domain.CreateRequestInput {
	return domain.CreateRequestInput{
		Category: "Barcode", Distributor: "PT Utama", Outlet: "Bandung Kota", SalesDivision: "M1 BIS",
		RequesterRole: "SA", RequesterName: "Laras P.", Qty: 3, Priority: "normal",
		CreatedBy: "u-submitter",
		Breakdown: []domain.QuantityBreakdownItem{{Kind: "Rusak", Quantity: 2}, {Kind: "Hilang", Quantity: 1}},
	}
}

// formService wires a service with the form master data above and a known requester.
func formService(t *testing.T) (domain.RequestService, *fakeRequestRepository, *fakeApprovalEngineClient) {
	t.Helper()
	repo := &fakeRequestRepository{
		nextID:     "REQ-NEW",
		requesters: map[string]domain.RequesterInfo{"Laras P.": {UserID: "u-laras", EmployeeNo: "EMP101", Name: "Laras P.", Email: "laras@x.co", ApprovalRank: 10}},
		usersByID: map[string]domain.RequesterInfo{
			"u-submitter": {UserID: "u-submitter", EmployeeNo: "EMP900", Name: "Sales Admin T", Email: "sa@x.co", ApprovalRank: 10},
			"u-laras":     {UserID: "u-laras", EmployeeNo: "EMP101", Name: "Laras P.", Email: "laras@x.co", ApprovalRank: 10},
		},
	}
	engine := &fakeApprovalEngineClient{}
	svc := NewRequestService(repo, engine, WithFormSource(fakeFormSource{data: formMasterData()}))
	return svc, repo, engine
}

func fieldOf(t *testing.T, err error) string {
	t.Helper()
	var ve *RequestValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want a *RequestValidationError", err)
	}
	if !errors.Is(err, ErrInvalidRequestPayload) {
		t.Fatalf("a validation error must still be an ErrInvalidRequestPayload")
	}
	return ve.Field
}

func TestCreateValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("a valid submission is stored with ids resolved from master data and names canonicalised", func(t *testing.T) {
		svc, repo, _ := formService(t)
		in := validInput()
		in.Outlet = "  bandung kota "
		in.SalesDivision = "m1 bis"
		in.Priority = "HIGH"
		if _, err := svc.Create(ctx, in); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := repo.createdInput
		if got.DistributorID != "d-utama" || got.OutletID != "o-bdg" {
			t.Errorf("resolved ids = %q / %q, want d-utama / o-bdg", got.DistributorID, got.OutletID)
		}
		if got.Outlet != "Bandung Kota" || got.SalesDivision != "M1 BIS" || got.Priority != "high" {
			t.Errorf("canonical values = %q / %q / %q", got.Outlet, got.SalesDivision, got.Priority)
		}
	})

	cases := []struct {
		name  string
		edit  func(*domain.CreateRequestInput)
		field string
	}{
		{"unknown category", func(i *domain.CreateRequestInput) { i.Category = "Laptop" }, "category"},
		{"no distributor at all", func(i *domain.CreateRequestInput) { i.Distributor = "" }, "distributor"},
		{"a distributor and a manual one together", func(i *domain.CreateRequestInput) { i.DistributorManual = "PT Baru" }, "distributor"},
		{"unknown distributor", func(i *domain.CreateRequestInput) { i.Distributor = "PT Hantu" }, "distributor"},
		{"manual distributor name too long", func(i *domain.CreateRequestInput) {
			i.Distributor, i.DistributorManual = "", strings.Repeat("x", 151)
		}, "distributorManual"},
		{"no outlet", func(i *domain.CreateRequestInput) { i.Outlet = "" }, "outlet"},
		{"an outlet of another distributor", func(i *domain.CreateRequestInput) { i.Outlet = "Surabaya Timur" }, "outlet"},
		{"an outlet that does not exist", func(i *domain.CreateRequestInput) { i.Outlet = "Atlantis" }, "outlet"},
		{"no sales division", func(i *domain.CreateRequestInput) { i.SalesDivision = "" }, "salesDivision"},
		{"unknown sales division", func(i *domain.CreateRequestInput) { i.SalesDivision = "M9" }, "salesDivision"},
		{"android without a request type", func(i *domain.CreateRequestInput) { i.Category, i.RequesterRole = "Android", "Cabang" }, "reqType"},
		{"android with an unknown request type", func(i *domain.CreateRequestInput) {
			i.Category, i.RequesterRole, i.ReqType = "Android", "Cabang", "Ganti"
		}, "reqType"},
		{"android requested by a sales supervisor", func(i *domain.CreateRequestInput) {
			i.Category, i.RequesterRole, i.ReqType = "Android", "SS", "Baru"
		}, "requesterRole"},
		{"server requested by a sales admin", func(i *domain.CreateRequestInput) { i.Category = "Server" }, "requesterRole"},
		{"barcode requested by a branch", func(i *domain.CreateRequestInput) { i.RequesterRole = "Cabang" }, "requesterRole"},
		{"no requester role", func(i *domain.CreateRequestInput) { i.RequesterRole = "" }, "requesterRole"},
		{"blank requester name", func(i *domain.CreateRequestInput) { i.RequesterName = "   " }, "requesterName"},
		{"zero quantity", func(i *domain.CreateRequestInput) { i.Qty = 0 }, "qty"},
		{"negative quantity", func(i *domain.CreateRequestInput) { i.Qty = -2 }, "qty"},
		{"unknown priority", func(i *domain.CreateRequestInput) { i.Priority = "critical" }, "priority"},
	}
	for _, c := range cases {
		t.Run("rejects: "+c.name, func(t *testing.T) {
			svc, repo, _ := formService(t)
			in := validInput()
			c.edit(&in)
			_, err := svc.Create(ctx, in)
			if got := fieldOf(t, err); got != c.field {
				t.Fatalf("field = %q, want %q (err: %v)", got, c.field, err)
			}
			if repo.createdInput.Category != "" {
				t.Fatal("nothing may be stored for a rejected submission")
			}
		})
	}

	t.Run("the first invalid field in form order is the one reported", func(t *testing.T) {
		svc, _, _ := formService(t)
		in := validInput()
		in.Qty, in.Priority, in.Outlet, in.Distributor = 0, "critical", "", ""
		_, err := svc.Create(ctx, in)
		if got := fieldOf(t, err); got != "distributor" {
			t.Fatalf("field = %q, want distributor (first on the form)", got)
		}
	})

	t.Run("an empty priority defaults to normal", func(t *testing.T) {
		svc, repo, _ := formService(t)
		in := validInput()
		in.Priority = ""
		if _, err := svc.Create(ctx, in); err != nil || repo.createdInput.Priority != "normal" {
			t.Fatalf("err %v, priority %q", err, repo.createdInput.Priority)
		}
	})

	t.Run("valid combinations are accepted", func(t *testing.T) {
		accepted := map[string]func(*domain.CreateRequestInput){
			"android by a branch": func(i *domain.CreateRequestInput) {
				i.Category, i.RequesterRole, i.ReqType = "Android", "Cabang", "Baru"
			},
			"android renewal by SD": func(i *domain.CreateRequestInput) {
				i.Category, i.RequesterRole, i.ReqType = "Android", "SD", "Peremajaan"
			},
			"server by a branch":      func(i *domain.CreateRequestInput) { i.Category, i.RequesterRole = "Server", "Cabang" },
			"barcode by the director": func(i *domain.CreateRequestInput) { i.RequesterRole = "SD" },
		}
		for name, edit := range accepted {
			svc, _, _ := formService(t)
			in := validInput()
			edit(&in)
			if _, err := svc.Create(ctx, in); err != nil {
				t.Errorf("%s: unexpected error %v", name, err)
			}
		}
	})

	t.Run("a request type is only kept for Android", func(t *testing.T) {
		svc, repo, _ := formService(t)
		in := validInput()
		in.ReqType = "Baru"
		if _, err := svc.Create(ctx, in); err != nil || repo.createdInput.ReqType != "" {
			t.Fatalf("err %v, stored request type %q; want it dropped for Barcode", err, repo.createdInput.ReqType)
		}
	})
}

func TestCreateManualDistributor(t *testing.T) {
	ctx := context.Background()

	t.Run("a typed distributor is kept as text and every outlet may be chosen", func(t *testing.T) {
		svc, repo, engine := formService(t)
		in := validInput()
		in.Distributor, in.DistributorManual, in.Outlet = "", "  PT Baru Jaya ", "Medan Kota" // Medan belongs to no distributor
		if _, err := svc.Create(ctx, in); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := repo.createdInput
		if got.DistributorID != "" || got.DistributorManual != "PT Baru Jaya" || got.OutletID != "o-mdn" {
			t.Errorf("stored distributor id %q, manual %q, outlet id %q", got.DistributorID, got.DistributorManual, got.OutletID)
		}
		if len(engine.createCalls) != 1 || engine.createCalls[0].Payload["distributor"] != "PT Baru Jaya" {
			t.Errorf("engine payload distributor = %v, want the typed name", engine.createCalls)
		}
	})

	t.Run("an outlet name shared by two outlets is ambiguous, not guessed", func(t *testing.T) {
		data := formMasterData()
		data.Outlets = append(data.Outlets, domain.Outlet{ID: "o-bdg2", Name: "Bandung Kota"})
		repo := &fakeRequestRepository{nextID: "REQ-NEW", requesters: map[string]domain.RequesterInfo{"Laras P.": {UserID: "u"}}}
		svc := NewRequestService(repo, &fakeApprovalEngineClient{}, WithFormSource(fakeFormSource{data: data}))
		in := validInput()
		in.Distributor, in.DistributorManual = "", "PT Baru"
		if got := fieldOf(t, func() error { _, err := svc.Create(ctx, in); return err }()); got != "outlet" {
			t.Fatalf("field = %q, want outlet", got)
		}
	})
}

func TestCreateRequesterIdentity(t *testing.T) {
	ctx := context.Background()

	t.Run("a name that matches a person is used as before and needs no remark", func(t *testing.T) {
		svc, repo, engine := formService(t)
		if _, err := svc.Create(ctx, validInput()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.createdRequester != "u-laras" || engine.createCalls[0].RequesterID != "EMP101" {
			t.Errorf("stored requester %q, engine requester %q; want u-laras / EMP101", repo.createdRequester, engine.createCalls[0].RequesterID)
		}
		for _, h := range repo.history["REQ-NEW"] {
			if h.Comment != nil {
				t.Errorf("unexpected remark %q on %q", *h.Comment, h.Action)
			}
		}
	})

	t.Run("a name that matches nobody is stored as typed, and the submitter stands in for the engine", func(t *testing.T) {
		svc, repo, engine := formService(t)
		in := validInput()
		in.RequesterName = "Cabang Bandung"
		if _, err := svc.Create(ctx, in); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.createdRequester != "u-submitter" || engine.createCalls[0].RequesterID != "EMP900" {
			t.Errorf("stored requester %q, engine requester %q; want the submitter u-submitter / EMP900", repo.createdRequester, engine.createCalls[0].RequesterID)
		}
		if repo.createdInput.RequesterName != "Cabang Bandung" {
			t.Errorf("typed name = %q, want it kept as typed", repo.createdInput.RequesterName)
		}
		var remark string
		for _, h := range repo.history["REQ-NEW"] {
			if h.Comment != nil {
				remark = *h.Comment
			}
		}
		if !strings.Contains(remark, "Cabang Bandung") {
			t.Errorf("history remark = %q, want it to name the typed requester", remark)
		}
	})

	t.Run("typing a person's email matches them, and is not an on-behalf-of submission", func(t *testing.T) {
		svc, repo, _ := formService(t)
		repo.requesters["laras@x.co"] = repo.requesters["Laras P."]
		in := validInput()
		in.RequesterName = "laras@x.co"
		if _, err := svc.Create(ctx, in); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, h := range repo.history["REQ-NEW"] {
			if h.Comment != nil {
				t.Errorf("unexpected remark %q", *h.Comment)
			}
		}
	})

	t.Run("an unknown name with no known submitter is rejected", func(t *testing.T) {
		svc, _, _ := formService(t)
		in := validInput()
		in.RequesterName, in.CreatedBy = "Nobody", ""
		if _, err := svc.Create(ctx, in); !errors.Is(err, ErrRequesterNotFound) {
			t.Fatalf("err = %v, want ErrRequesterNotFound", err)
		}
	})

	t.Run("the engine is told the chosen requester role as well", func(t *testing.T) {
		svc, _, engine := formService(t)
		if _, err := svc.Create(ctx, validInput()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		p := engine.createCalls[0].Payload
		if p["requesterRole"] != "SA" || p["requesterApprovalRank"] != 10 {
			t.Errorf("payload requesterRole %v, requesterApprovalRank %v; want SA and the user's own rank 10", p["requesterRole"], p["requesterApprovalRank"])
		}
	})
}

func TestFormOptions(t *testing.T) {
	ctx := context.Background()

	t.Run("the form is built from active master data", func(t *testing.T) {
		data := formMasterData()
		data.Distributors[0].Outlets = append(data.Distributors[0].Outlets, domain.OutletRef{ID: "o-off", Name: "Closed Outlet"}) // not in the active outlet list
		svc := NewRequestService(&fakeRequestRepository{}, &fakeApprovalEngineClient{}, WithFormSource(fakeFormSource{data: data}))

		got, err := svc.FormOptions(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.Distributors) != 2 || len(got.Distributors[0].Outlets) != 2 {
			t.Fatalf("distributors = %+v, want 2 and the inactive outlet dropped from PT Utama", got.Distributors)
		}
		if len(got.Outlets) != 4 || len(got.SalesDivisions) != 2 {
			t.Errorf("outlets %d, sales divisions %d; want 4 and 2", len(got.Outlets), len(got.SalesDivisions))
		}
		if strings.Join(got.Categories, ",") != "Barcode,Android,Server,Mobile Printer" || strings.Join(got.RequestTypes, ",") != "Baru,Peremajaan" || strings.Join(got.Priorities, ",") != "normal,high,urgent" {
			t.Errorf("categories %v, request types %v, priorities %v", got.Categories, got.RequestTypes, got.Priorities)
		}
	})

	t.Run("requester roles follow the document: six levels for barcode, two otherwise", func(t *testing.T) {
		svc := NewRequestService(&fakeRequestRepository{}, &fakeApprovalEngineClient{}, WithFormSource(fakeFormSource{data: formMasterData()}))
		got, _ := svc.FormOptions(ctx)
		codes := func(category string) string {
			var out []string
			for _, r := range got.RequesterRoles[category] {
				out = append(out, r.Code)
			}
			return strings.Join(out, ",")
		}
		if codes("Barcode") != "SA,SS,RSM,GRSM,NSM,SD" || codes("Android") != "Cabang,SD" || codes("Server") != "Cabang,SD" {
			t.Errorf("barcode %q, android %q, server %q", codes("Barcode"), codes("Android"), codes("Server"))
		}
		if got.RequesterRoles["Barcode"][0].Label != "Sales Admin" || got.RequesterRoles["Android"][0].Label != "Cabang / Distributor" {
			t.Errorf("labels = %+v", got.RequesterRoles)
		}
	})

	t.Run("without a master data source the options cannot be built", func(t *testing.T) {
		svc := NewRequestService(&fakeRequestRepository{}, &fakeApprovalEngineClient{})
		if _, err := svc.FormOptions(ctx); !errors.Is(err, ErrFormOptionsUnavailable) {
			t.Fatalf("err = %v, want ErrFormOptionsUnavailable", err)
		}
	})

	t.Run("a master data failure is returned", func(t *testing.T) {
		boom := errors.New("database unavailable")
		svc := NewRequestService(&fakeRequestRepository{}, &fakeApprovalEngineClient{}, WithFormSource(fakeFormSource{err: boom}))
		if _, err := svc.FormOptions(ctx); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the master data error", err)
		}
	})
}

func barcodeWith(items ...domain.QuantityBreakdownItem) domain.CreateRequestInput {
	in := validInput()
	in.Breakdown = items
	total := 0
	for _, i := range items {
		total += i.Quantity
	}
	in.Qty = total
	return in
}

func TestBarcodeBreakdown(t *testing.T) {
	ctx := context.Background()
	kind := func(k string, q int) domain.QuantityBreakdownItem {
		return domain.QuantityBreakdownItem{Kind: k, Quantity: q}
	}

	t.Run("the total is the sum of the kinds, and zero counts are not stored", func(t *testing.T) {
		svc, repo, _ := formService(t)
		in := barcodeWith(kind("Rusak", 2), kind("Hilang", 0), kind("Buffer Stock", 3))
		if _, err := svc.Create(ctx, in); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := repo.createdInput
		if got.Qty != 5 || len(got.Breakdown) != 2 || got.Breakdown[0].Kind != "Rusak" || got.Breakdown[1].Kind != "Buffer Stock" {
			t.Fatalf("stored qty %d, breakdown %+v; want 5 with Rusak and Buffer Stock only", got.Qty, got.Breakdown)
		}
	})

	t.Run("kind names are matched ignoring case and stored as the document spells them", func(t *testing.T) {
		svc, repo, _ := formService(t)
		if _, err := svc.Create(ctx, barcodeWith(kind(" noo ", 1), kind("buffer stock", 1))); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.createdInput.Breakdown[0].Kind != "NOO" || repo.createdInput.Breakdown[1].Kind != "Buffer Stock" {
			t.Fatalf("breakdown = %+v", repo.createdInput.Breakdown)
		}
	})

	rejected := []struct {
		name  string
		in    domain.CreateRequestInput
		field string
	}{
		{"no breakdown at all", func() domain.CreateRequestInput { in := validInput(); in.Breakdown = nil; return in }(), "breakdown"},
		{"every count zero", barcodeWith(kind("Rusak", 0), kind("Hilang", 0)), "breakdown"},
		{"an unknown kind", barcodeWith(kind("Curian", 2)), "breakdown"},
		{"a negative count", func() domain.CreateRequestInput {
			in := barcodeWith(kind("Rusak", 3))
			in.Breakdown[0].Quantity = -1
			return in
		}(), "breakdown"},
		{"the same kind twice", barcodeWith(kind("Rusak", 1), kind("rusak", 1)), "breakdown"},
		{"a total that is not the sum", func() domain.CreateRequestInput {
			in := barcodeWith(kind("Rusak", 2), kind("Hilang", 2))
			in.Qty = 9
			return in
		}(), "qty"},
	}
	for _, c := range rejected {
		t.Run("rejects: "+c.name, func(t *testing.T) {
			svc, repo, _ := formService(t)
			_, err := svc.Create(ctx, c.in)
			if got := fieldOf(t, err); got != c.field {
				t.Fatalf("field = %q, want %q (err: %v)", got, c.field, err)
			}
			if repo.createdInput.Category != "" {
				t.Fatal("nothing may be stored")
			}
		})
	}

	t.Run("other categories take a plain quantity and drop any breakdown", func(t *testing.T) {
		svc, repo, _ := formService(t)
		in := validInput()
		in.Category, in.RequesterRole, in.ReqType = "Android", "Cabang", "Baru"
		in.Breakdown = []domain.QuantityBreakdownItem{kind("Rusak", 1)}
		if _, err := svc.Create(ctx, in); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.createdInput.Qty != 3 || len(repo.createdInput.Breakdown) != 0 {
			t.Fatalf("qty %d, breakdown %+v; want 3 and none", repo.createdInput.Qty, repo.createdInput.Breakdown)
		}
	})

	t.Run("the form offers the four kinds", func(t *testing.T) {
		svc, _, _ := formService(t)
		got, _ := svc.FormOptions(ctx)
		if strings.Join(got.BarcodeKinds, ",") != "Rusak,Hilang,NOO,Buffer Stock" {
			t.Fatalf("kinds = %v", got.BarcodeKinds)
		}
	})
}

func TestMobilePrinter(t *testing.T) {
	ctx := context.Background()
	printer := func() domain.CreateRequestInput {
		in := validInput()
		in.Category, in.RequesterRole = "Mobile Printer", "Cabang"
		return in
	}

	t.Run("it is a fourth category, requested by a branch or the Sales Director like Server", func(t *testing.T) {
		svc, _, _ := formService(t)
		got, _ := svc.FormOptions(ctx)
		if strings.Join(got.Categories, ",") != "Barcode,Android,Server,Mobile Printer" {
			t.Fatalf("categories = %v", got.Categories)
		}
		var roles []string
		for _, r := range got.RequesterRoles["Mobile Printer"] {
			roles = append(roles, r.Code)
		}
		if strings.Join(roles, ",") != "Cabang,SD" {
			t.Fatalf("roles = %v", roles)
		}
	})

	t.Run("a new request needs no request type, and any type sent is dropped", func(t *testing.T) {
		svc, repo, _ := formService(t)
		in := printer()
		in.ReqType = "Peremajaan" // there is no replacement for a Mobile Printer
		if _, err := svc.Create(ctx, in); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.createdInput.ReqType != "" {
			t.Fatalf("request type = %q, want none", repo.createdInput.ReqType)
		}
	})

	t.Run("a sales level cannot request it", func(t *testing.T) {
		svc, _, _ := formService(t)
		in := printer()
		in.RequesterRole = "SS"
		if got := fieldOf(t, func() error { _, err := svc.Create(ctx, in); return err }()); got != "requesterRole" {
			t.Fatalf("field = %q", got)
		}
	})

	t.Run("it follows the same Approval Engine workflow as Server", func(t *testing.T) {
		svc, _, engine := formService(t)
		if _, err := svc.Create(ctx, printer()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(engine.createCalls) != 1 || engine.createCalls[0].DocType != "asset_request_field_device" {
			t.Fatalf("engine calls = %+v, want one with doc_type asset_request_field_device", engine.createCalls)
		}
	})
}

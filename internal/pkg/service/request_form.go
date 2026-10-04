package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

// ErrFormOptionsUnavailable: the service was built without a master data source.
var ErrFormOptionsUnavailable = errors.New("request form options are not available")

// RequestValidationError says which form field is wrong, so the form can mark
// that field. It is an ErrInvalidRequestPayload for callers that only care
// that the submission was rejected.
type RequestValidationError struct {
	Field   string // the form field's name: category, distributor, outlet, ...
	Message string
}

func (e *RequestValidationError) Error() string { return e.Message }
func (e *RequestValidationError) Unwrap() error { return ErrInvalidRequestPayload }

func invalidField(field, format string, args ...any) error {
	return &RequestValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

// RequestServiceOption configures NewRequestService.
type RequestServiceOption func(*requestService)

// WithFormSource gives the service the master data behind the New Request form.
// Production wiring always passes it. Without it, Create still enforces every
// rule that needs no master data (category, request type, requester role,
// quantity and so on) but cannot check or resolve distributor, outlet or sales
// division, and FormOptions is unavailable.
func WithFormSource(source domain.RequestFormSource) RequestServiceOption {
	return func(s *requestService) { s.forms = source }
}

const maxManualDistributorLength = 150

var (
	requestCategories = []string{domain.CategoryBarcode, domain.CategoryAndroid, domain.CategoryServer, domain.CategoryMobilePrinter}
	// barcodeKinds are the Tipe Pengajuan of a Barcode request, in the order the document lists them.
	barcodeKinds      = []string{domain.BarcodeKindDamaged, domain.BarcodeKindLost, domain.BarcodeKindNewOutlet, domain.BarcodeKindBufferStock}
	requestTypes      = []string{domain.RequestTypeNew, domain.RequestTypeRenewal}
	requestPriorities = []string{domain.PriorityNormal, domain.PriorityHigh, domain.PriorityUrgent}
)

var requesterRoleLabels = map[string]string{
	"SA":     "Sales Admin",
	"SS":     "Sales Supervisor",
	"RSM":    "Regional Sales Manager",
	"GRSM":   "Group Regional Sales Manager",
	"NSM":    "National Sales Manager",
	"SD":     "Sales Director",
	"Cabang": "Cabang / Distributor",
}

// requesterRolesByCategory: Barcode may be requested at every sales level;
// Android and Server only by a branch or by the Sales Director.
var requesterRolesByCategory = map[string][]string{
	domain.CategoryBarcode: {"SA", "SS", "RSM", "GRSM", "NSM", "SD"},
	domain.CategoryAndroid: {"Cabang", "SD"},
	domain.CategoryServer:  {"Cabang", "SD"},
	// Mobile Printer follows Server (a branch or the Sales Director) until the business fixes its hierarchy.
	domain.CategoryMobilePrinter: {"Cabang", "SD"},
}

func containsFold(list []string, value string) (string, bool) {
	for _, item := range list {
		if strings.EqualFold(item, value) {
			return item, true
		}
	}
	return "", false
}

// FormOptions returns what the New Request form offers.
func (s *requestService) FormOptions(ctx context.Context) (*domain.RequestFormOptions, error) {
	if s.forms == nil {
		return nil, ErrFormOptionsUnavailable
	}
	data, err := s.forms.FormMasterData(ctx)
	if err != nil {
		return nil, err
	}

	active := make(map[string]bool, len(data.Outlets))
	outlets := make([]domain.OutletRef, 0, len(data.Outlets))
	for _, o := range data.Outlets {
		active[o.ID] = true
		outlets = append(outlets, domain.OutletRef{ID: o.ID, Name: o.Name})
	}
	distributors := make([]domain.Distributor, 0, len(data.Distributors))
	for _, d := range data.Distributors {
		kept := make([]domain.OutletRef, 0, len(d.Outlets))
		for _, o := range d.Outlets {
			if active[o.ID] {
				kept = append(kept, o) // a deactivated outlet is no longer offered
			}
		}
		d.Outlets = kept
		distributors = append(distributors, d)
	}

	roles := make(map[string][]domain.RequesterRoleOption, len(requesterRolesByCategory))
	for category, codes := range requesterRolesByCategory {
		for _, code := range codes {
			roles[category] = append(roles[category], domain.RequesterRoleOption{Code: code, Label: requesterRoleLabels[code]})
		}
	}
	return &domain.RequestFormOptions{
		Categories:     requestCategories,
		Distributors:   distributors,
		Outlets:        outlets,
		SalesDivisions: data.SalesDivisions,
		RequestTypes:   requestTypes,
		BarcodeKinds:   barcodeKinds,
		Priorities:     requestPriorities,
		RequesterRoles: roles,
	}, nil
}

// validateCreate checks a submission and returns it normalised: names trimmed
// and spelled as in master data, ids of the distributor and outlet filled in,
// priority lower-cased. Fields are checked in the order they appear on the
// form, so the error always names the first field the user should fix.
// data is nil when the service has no master data source (see WithFormSource).
func validateCreate(in domain.CreateRequestInput, data *domain.FormMasterData) (domain.CreateRequestInput, error) {
	// Category
	category, ok := containsFold(requestCategories, strings.TrimSpace(in.Category))
	if !ok {
		return in, invalidField("category", "Kategori request tidak valid.")
	}
	in.Category = category

	// Distributor: one from master data, or one typed in by hand, never both.
	in.Distributor = strings.TrimSpace(in.Distributor)
	in.DistributorManual = strings.TrimSpace(in.DistributorManual)
	switch {
	case in.Distributor == "" && in.DistributorManual == "":
		return in, invalidField("distributor", "Distributor wajib diisi.")
	case in.Distributor != "" && in.DistributorManual != "":
		return in, invalidField("distributor", "Pilih distributor dari daftar atau isi manual, tidak keduanya.")
	case len([]rune(in.DistributorManual)) > maxManualDistributorLength:
		return in, invalidField("distributorManual", "Nama distributor maksimal %d karakter.", maxManualDistributorLength)
	}

	var distributor *domain.Distributor
	if data != nil && in.Distributor != "" {
		for i := range data.Distributors {
			if strings.EqualFold(data.Distributors[i].Name, in.Distributor) {
				distributor = &data.Distributors[i]
				break
			}
		}
		if distributor == nil {
			return in, invalidField("distributor", "Distributor %q tidak ada di master data.", in.Distributor)
		}
		in.Distributor, in.DistributorID = distributor.Name, distributor.ID
	}

	// Outlet: those of the chosen distributor, or every outlet for a manual one.
	in.Outlet = strings.TrimSpace(in.Outlet)
	if in.Outlet == "" {
		return in, invalidField("outlet", "Outlet wajib dipilih.")
	}
	if data != nil {
		candidates := make([]domain.OutletRef, 0, len(data.Outlets))
		if distributor != nil {
			active := make(map[string]bool, len(data.Outlets))
			for _, o := range data.Outlets {
				active[o.ID] = true
			}
			for _, o := range distributor.Outlets {
				if active[o.ID] {
					candidates = append(candidates, o)
				}
			}
		} else {
			for _, o := range data.Outlets {
				candidates = append(candidates, domain.OutletRef{ID: o.ID, Name: o.Name})
			}
		}
		var matches []domain.OutletRef
		for _, o := range candidates {
			if strings.EqualFold(o.Name, in.Outlet) {
				matches = append(matches, o)
			}
		}
		switch len(matches) {
		case 0:
			if distributor != nil {
				return in, invalidField("outlet", "Outlet %q tidak dilayani distributor %s.", in.Outlet, distributor.Name)
			}
			return in, invalidField("outlet", "Outlet %q tidak ada di master data.", in.Outlet)
		case 1:
			in.Outlet, in.OutletID = matches[0].Name, matches[0].ID
		default:
			return in, invalidField("outlet", "Ada lebih dari satu outlet bernama %q; hubungi Admin untuk merapikan master data.", in.Outlet)
		}
	}

	// Sales division
	in.SalesDivision = strings.TrimSpace(in.SalesDivision)
	if in.SalesDivision == "" {
		return in, invalidField("salesDivision", "Sales Division wajib dipilih.")
	}
	if data != nil {
		division, found := containsFold(data.SalesDivisions, in.SalesDivision)
		if !found {
			return in, invalidField("salesDivision", "Sales Division %q tidak ada di master data.", in.SalesDivision)
		}
		in.SalesDivision = division
	}

	// Request type: asked for Android only, dropped for everything else.
	if in.Category == domain.CategoryAndroid {
		reqType, found := containsFold(requestTypes, strings.TrimSpace(in.ReqType))
		if !found {
			return in, invalidField("reqType", "Request Type wajib dipilih untuk kategori Android (Baru atau Peremajaan).")
		}
		in.ReqType = reqType
	} else {
		in.ReqType = ""
	}

	// Requester role depends on the category.
	role, found := containsFold(requesterRolesByCategory[in.Category], strings.TrimSpace(in.RequesterRole))
	if !found {
		return in, invalidField("requesterRole", "Requester Role tidak sesuai untuk kategori %s.", in.Category)
	}
	in.RequesterRole = role

	in.RequesterName = strings.TrimSpace(in.RequesterName)
	if in.RequesterName == "" {
		return in, invalidField("requesterName", "Nama requester wajib diisi.")
	}

	// Quantity. A Barcode request counts its units by reason (Tipe Pengajuan) and
	// its total is their sum; the server holds the client to that. Every other
	// category just has a quantity.
	if in.Category == domain.CategoryBarcode {
		items, total, err := normalizeBreakdown(in.Breakdown)
		if err != nil {
			return in, err
		}
		if in.Qty != total {
			return in, invalidField("qty", "Total Request harus sama dengan jumlah semua Tipe Pengajuan (%d).", total)
		}
		in.Breakdown = items
	} else {
		in.Breakdown = nil
		if in.Qty < 1 {
			return in, invalidField("qty", "Quantity minimal 1.")
		}
	}

	if strings.TrimSpace(in.Priority) == "" {
		in.Priority = domain.PriorityNormal
	}
	priority, found := containsFold(requestPriorities, strings.TrimSpace(in.Priority))
	if !found {
		return in, invalidField("priority", "Priority harus Normal, High, atau Urgent.")
	}
	in.Priority = priority

	return in, nil
}

// distributorLabel is the distributor as people read it: the master data name,
// or the one typed in by hand.
func distributorLabel(in domain.CreateRequestInput) string {
	if in.DistributorManual != "" {
		return in.DistributorManual
	}
	return in.Distributor
}

// standsIn reports whether requester is merely standing in for the name that
// was typed on the form (the submitter, when nobody matches that name), rather
// than being that person.
func standsIn(typedName string, requester *domain.RequesterInfo) bool {
	typedName = strings.TrimSpace(typedName)
	return typedName != "" && !strings.EqualFold(typedName, requester.Name) && !strings.EqualFold(typedName, requester.Email)
}

// resolveRequester decides whose identity the request carries into the
// Approval Engine, which routes by that person's place in the org chart.
//
//   - The typed name matches a registered person: that person, as always.
//   - It matches nobody (a branch, say: the name is free text): the user who is
//     submitting, who is always a known person. The history records the typed
//     name, so approvers can see on whose behalf the request was made.
//   - It matches nobody and the submitter is unknown: rejected.
func (s *requestService) resolveRequester(ctx context.Context, in domain.CreateRequestInput) (*domain.RequesterInfo, error) {
	requester, err := s.repo.ResolveRequester(ctx, in.RequesterName)
	if err != nil {
		return nil, err
	}
	if requester != nil {
		return requester, nil
	}
	if in.CreatedBy == "" {
		return nil, ErrRequesterNotFound
	}
	submitter, err := s.repo.ResolveRequesterByUserID(ctx, in.CreatedBy)
	if err != nil {
		return nil, err
	}
	if submitter == nil {
		return nil, ErrRequesterNotFound
	}
	return submitter, nil
}

// normalizeBreakdown checks a Barcode request's Tipe Pengajuan counts and
// returns the kinds with a count above zero, in the document's order, and their
// total. Kind names are matched ignoring case; a kind may appear once.
func normalizeBreakdown(items []domain.QuantityBreakdownItem) ([]domain.QuantityBreakdownItem, int, error) {
	counts := make(map[string]int, len(barcodeKinds))
	for _, item := range items {
		kind, ok := containsFold(barcodeKinds, strings.TrimSpace(item.Kind))
		if !ok {
			return nil, 0, invalidField("breakdown", "Tipe Pengajuan %q tidak dikenal (pilih %s).", item.Kind, strings.Join(barcodeKinds, ", "))
		}
		if item.Quantity < 0 {
			return nil, 0, invalidField("breakdown", "Jumlah untuk %s tidak boleh negatif.", kind)
		}
		if _, dup := counts[kind]; dup {
			return nil, 0, invalidField("breakdown", "Tipe Pengajuan %s diisi lebih dari sekali.", kind)
		}
		counts[kind] = item.Quantity
	}

	var kept []domain.QuantityBreakdownItem
	total := 0
	for _, kind := range barcodeKinds {
		if n := counts[kind]; n > 0 {
			kept = append(kept, domain.QuantityBreakdownItem{Kind: kind, Quantity: n})
			total += n
		}
	}
	if total == 0 {
		return nil, 0, invalidField("breakdown", "Isi jumlah untuk minimal satu Tipe Pengajuan.")
	}
	return kept, total, nil
}

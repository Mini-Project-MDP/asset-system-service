package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Errors for the fulfillment stage (Processing -> Shipped -> Delivered -> Completed).
var (
	// ErrFulfillmentNotReady: the request is not in a state that allows the
	// attempted step (still waiting for approval, data not recorded yet, or
	// already completed). The handler answers 409.
	ErrFulfillmentNotReady = errors.New("request is not ready for this fulfillment step")
	// ErrFulfillmentDataInvalid: the asset data does not satisfy the rules of
	// the request's category. The handler answers 400 with the reason.
	ErrFulfillmentDataInvalid = errors.New("invalid fulfillment data")
)

// IMEI numbers are 15 digits; 14 (without check digit) and 16 (IMEISV) are
// accepted too, since scanners and CSV exports produce all three.
const (
	imeiMinLen = 14
	imeiMaxLen = 16
)

func invalidData(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrFulfillmentDataInvalid, fmt.Sprintf(format, args...))
}

type barcodeData struct {
	Codes []string `json:"codes"`
}

// serialData is the data of a Mobile Printer request: one serial number per unit.
type serialData struct {
	Serials []string `json:"serials"`
}

type androidUnit struct {
	IMEI        string `json:"imei"`
	Brand       string `json:"brand"`
	Model       string `json:"model"`
	ReleaseYear string `json:"releaseYear"`
	RegYear     string `json:"regYear"`
}

type androidData struct {
	Units []androidUnit `json:"units"`
}

type serverData struct {
	Specs string `json:"specs"`
}

// validateFulfillmentData checks the asset data recorded at Processing against
// the rules of the request's category and returns it as normalized JSON (the
// form stored in asset_requests.fulfillment_data):
//
//   - Barcode: exactly qty non-empty codes, no duplicates.
//   - Android: exactly qty units, each with IMEI (14-16 digits, no duplicates),
//     brand, model and two 4-digit years.
//   - Server: a non-empty free-text spec, recorded once for the whole request.
//   - Mobile Printer: exactly qty serial numbers, none empty, no duplicates.
func validateFulfillmentData(category string, qty int, raw string) (string, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &object); err != nil || object == nil {
		return "", invalidData("fulfillData must be a JSON object")
	}

	switch category {
	case "Barcode":
		return validateBarcodeData(qty, raw)
	case "Android":
		return validateAndroidData(qty, raw)
	case "Server":
		return validateServerData(raw)
	case "Mobile Printer":
		return validateSerialData(qty, raw)
	default:
		return "", invalidData("unsupported category %q", category)
	}
}

func marshalNormalized(v any) (string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encode fulfillment data: %w", err)
	}
	return string(out), nil
}

func validateBarcodeData(qty int, raw string) (string, error) {
	var data barcodeData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return "", invalidData("codes must be a list of strings")
	}
	if err := cleanUnitCodes(data.Codes, qty, "barcode code"); err != nil {
		return "", err
	}
	return marshalNormalized(data)
}

func validateSerialData(qty int, raw string) (string, error) {
	var data serialData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return "", invalidData("serials must be a list of serial numbers (text)")
	}
	if err := cleanUnitCodes(data.Serials, qty, "serial number"); err != nil {
		return "", err
	}
	return marshalNormalized(data)
}

// cleanUnitCodes checks one code per unit (barcode codes, serial numbers): the
// right count, none empty, none repeated. It trims the codes in place. what
// names the kind of code in the messages.
func cleanUnitCodes(codes []string, qty int, what string) error {
	if len(codes) != qty {
		return invalidData("expected %d %ss, got %d", qty, what, len(codes))
	}
	seen := make(map[string]int, qty)
	for i, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			return invalidData("%s %d of %d is empty", what, i+1, qty)
		}
		if first, dup := seen[code]; dup {
			return invalidData("duplicate %s %q (rows %d and %d)", what, code, first, i+1)
		}
		seen[code] = i + 1
		codes[i] = code
	}
	return nil
}

func isFourDigitYear(s string) bool {
	if len(s) != 4 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func validateAndroidData(qty int, raw string) (string, error) {
	var data androidData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return "", invalidData("units must be a list of unit objects with text fields")
	}
	if len(data.Units) != qty {
		return "", invalidData("expected %d units, got %d", qty, len(data.Units))
	}
	seen := make(map[string]int, qty)
	for i := range data.Units {
		u := &data.Units[i]
		n := i + 1
		u.IMEI = strings.TrimSpace(u.IMEI)
		u.Brand = strings.TrimSpace(u.Brand)
		u.Model = strings.TrimSpace(u.Model)
		u.ReleaseYear = strings.TrimSpace(u.ReleaseYear)
		u.RegYear = strings.TrimSpace(u.RegYear)

		if !isDigits(u.IMEI) || len(u.IMEI) < imeiMinLen || len(u.IMEI) > imeiMaxLen {
			return "", invalidData("unit %d: IMEI must be %d-%d digits", n, imeiMinLen, imeiMaxLen)
		}
		if first, dup := seen[u.IMEI]; dup {
			return "", invalidData("duplicate IMEI %s (units %d and %d)", u.IMEI, first, n)
		}
		seen[u.IMEI] = n
		if u.Brand == "" {
			return "", invalidData("unit %d: brand is required", n)
		}
		if u.Model == "" {
			return "", invalidData("unit %d: model is required", n)
		}
		if !isFourDigitYear(u.ReleaseYear) {
			return "", invalidData("unit %d: releaseYear must be a 4-digit year", n)
		}
		if !isFourDigitYear(u.RegYear) {
			return "", invalidData("unit %d: regYear must be a 4-digit year", n)
		}
	}
	return marshalNormalized(data)
}

func validateServerData(raw string) (string, error) {
	var data serverData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return "", invalidData("specs must be text")
	}
	data.Specs = strings.TrimSpace(data.Specs)
	if data.Specs == "" {
		return "", invalidData("specs is required")
	}
	return marshalNormalized(data)
}

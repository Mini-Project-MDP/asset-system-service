package domain

import "context"

// ImeiInfo is the device a TAC (the first 8 digits of an IMEI) belongs to.
type ImeiInfo struct {
	Brand       string
	Model       string
	ReleaseYear string
}

// ImeiReferenceRepository looks devices up in the TAC reference table.
// LookupTAC returns (nil, nil) when the TAC is not in the table.
type ImeiReferenceRepository interface {
	LookupTAC(ctx context.Context, tac string) (*ImeiInfo, error)
}

// ImeiService identifies a device from its IMEI, to pre-fill brand, model and
// release year in the Android fulfillment form.
type ImeiService interface {
	// Lookup returns (nil, nil) when the device is unknown. That is a normal
	// outcome (the form falls back to manual input), not an error. A value
	// that is not a plausible IMEI is an error.
	Lookup(ctx context.Context, imei string) (*ImeiInfo, error)
}

// ImeiReferenceRow is one line of the TAC reference table.
type ImeiReferenceRow struct {
	TAC         string // first 8 digits of an IMEI
	Brand       string
	Model       string
	ReleaseYear string // 4-digit year
}

package domain

import (
	"context"
	"time"
)

// DashboardStats are the four KPI numbers on the dashboard.
type DashboardStats struct {
	// RequestsThisMonth: requests created in the current calendar month (WIB).
	RequestsThisMonth int
	// PendingApproval: requests waiting for someone's approval.
	PendingApproval int
	// InProgress: approved requests being fulfilled (Processing through Delivered).
	InProgress int
	// AssetsRegistered: units on requests whose asset data has been recorded
	// at Fulfillment. There is no asset registry yet, so this is the closest
	// thing the system knows.
	AssetsRegistered int
}

// CategoryMonthCount is one cell of the monthly chart: how many requests of a
// category (an asset type code) were created in a month (1-12, WIB).
type CategoryMonthCount struct {
	Month    int
	Category string
	Count    int
}

// MonthlyRequestCount is one month on the chart, split by category.
type MonthlyRequestCount struct {
	Month         int // 1-12
	Barcode       int
	MobilePrinter int
	Android       int
	Server        int
}

// ActivityRecord is a raw request_history event.
type ActivityRecord struct {
	ID        string
	RequestID string
	Actor     string // the actor's name when known, otherwise the stored role/employee number
	Action    string
	CreatedAt time.Time
}

// DashboardActivity is an activity feed entry, ready to display.
type DashboardActivity struct {
	ID      string
	Title   string
	TimeAgo string // "now", "5m", "2h", "3d"
	Type    string // approval, request, fulfillment or warning
}

// DashboardOverview is everything the dashboard shows.
type DashboardOverview struct {
	Stats DashboardStats
	// Month is the current month (WIB), the period the first KPI counts.
	Month      time.Month
	Year       int
	Months     []MonthlyRequestCount
	Activities []DashboardActivity
	Attention  []AssetRequest // oldest, most urgent requests waiting for approval
}

// DashboardRepository reads the aggregates behind the dashboard.
type DashboardRepository interface {
	// Stats counts requests; the month bounds delimit "this month" (start inclusive, end exclusive).
	Stats(ctx context.Context, monthStart, monthEnd time.Time) (DashboardStats, error)
	// CategoryMonthCounts counts requests created in [yearStart, yearEnd) by month and category.
	CategoryMonthCounts(ctx context.Context, yearStart, yearEnd time.Time) ([]CategoryMonthCount, error)
	// RecentActivity returns the newest history events across all requests.
	RecentActivity(ctx context.Context, limit int) ([]ActivityRecord, error)
	// AttentionRequests returns requests waiting for approval with their
	// approval chain: urgent first, then high, then normal, oldest first.
	AttentionRequests(ctx context.Context, limit int) ([]AssetRequest, error)
}

// DashboardService builds the dashboard overview.
type DashboardService interface {
	// Overview assembles the dashboard for a year (0 means the current year).
	Overview(ctx context.Context, year int) (*DashboardOverview, error)
}

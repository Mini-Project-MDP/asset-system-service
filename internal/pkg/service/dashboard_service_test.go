package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

// fakeDashboardRepository records what the service asked for and replays canned data.
type fakeDashboardRepository struct {
	stats      domain.DashboardStats
	counts     []domain.CategoryMonthCount
	activity   []domain.ActivityRecord
	attention  []domain.AssetRequest
	failWith   error
	monthStart time.Time
	monthEnd   time.Time
	yearStart  time.Time
	yearEnd    time.Time
	activityN  int
	attentionN int
}

func (f *fakeDashboardRepository) Stats(ctx context.Context, monthStart, monthEnd time.Time) (domain.DashboardStats, error) {
	f.monthStart, f.monthEnd = monthStart, monthEnd
	return f.stats, f.failWith
}

func (f *fakeDashboardRepository) CategoryMonthCounts(ctx context.Context, yearStart, yearEnd time.Time) ([]domain.CategoryMonthCount, error) {
	f.yearStart, f.yearEnd = yearStart, yearEnd
	return f.counts, nil
}

func (f *fakeDashboardRepository) RecentActivity(ctx context.Context, limit int) ([]domain.ActivityRecord, error) {
	f.activityN = limit
	return f.activity, nil
}

func (f *fakeDashboardRepository) AttentionRequests(ctx context.Context, limit int) ([]domain.AssetRequest, error) {
	f.attentionN = limit
	return f.attention, nil
}

// wib is the business time zone (UTC+7); month and year boundaries follow it.
var wib = time.FixedZone("WIB", 7*3600)

func newDashboard(repo *fakeDashboardRepository, now time.Time) domain.DashboardService {
	return NewDashboardService(repo, func() time.Time { return now })
}

func TestDashboardYear(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, wib)

	t.Run("year 0 means the current year", func(t *testing.T) {
		got, err := newDashboard(&fakeDashboardRepository{}, now).Overview(ctx, 0)
		if err != nil || got.Year != 2026 || got.Month != time.October {
			t.Fatalf("got %+v, err = %v, want year 2026 and month October", got, err)
		}
	})

	t.Run("a year outside 2000-2100 is rejected", func(t *testing.T) {
		for _, y := range []int{-1, 1999, 2101} {
			if _, err := newDashboard(&fakeDashboardRepository{}, now).Overview(ctx, y); !errors.Is(err, ErrInvalidDashboardYear) {
				t.Errorf("year %d: err = %v, want ErrInvalidDashboardYear", y, err)
			}
		}
	})

	t.Run("the current year runs up to the current month only", func(t *testing.T) {
		got, _ := newDashboard(&fakeDashboardRepository{}, now).Overview(ctx, 2026)
		if len(got.Months) != 10 || got.Months[0].Month != 1 || got.Months[9].Month != 10 {
			t.Fatalf("months = %+v, want January to October", got.Months)
		}
	})

	t.Run("a past year has 12 months and a future year has none", func(t *testing.T) {
		past, _ := newDashboard(&fakeDashboardRepository{}, now).Overview(ctx, 2025)
		future, _ := newDashboard(&fakeDashboardRepository{}, now).Overview(ctx, 2027)
		if len(past.Months) != 12 || len(future.Months) != 0 {
			t.Fatalf("past has %d months, future has %d; want 12 and 0", len(past.Months), len(future.Months))
		}
	})

	t.Run("the year bounds are asked for in WIB", func(t *testing.T) {
		repo := &fakeDashboardRepository{}
		newDashboard(repo, now).Overview(ctx, 2025)
		wantStart := time.Date(2025, 1, 1, 0, 0, 0, 0, wib)
		wantEnd := time.Date(2026, 1, 1, 0, 0, 0, 0, wib)
		if !repo.yearStart.Equal(wantStart) || !repo.yearEnd.Equal(wantEnd) {
			t.Fatalf("bounds = %v .. %v, want %v .. %v", repo.yearStart, repo.yearEnd, wantStart, wantEnd)
		}
	})
}

func TestDashboardMonthBoundaryFollowsWIB(t *testing.T) {
	// 2026-12-31 23:30 UTC is already 2027-01-01 06:30 in WIB: "this month" is January 2027.
	now := time.Date(2026, 12, 31, 23, 30, 0, 0, time.UTC)
	repo := &fakeDashboardRepository{}
	got, err := newDashboard(repo, now).Overview(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Year != 2027 {
		t.Fatalf("year = %d, want 2027", got.Year)
	}
	if !repo.monthStart.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, wib)) || !repo.monthEnd.Equal(time.Date(2027, 2, 1, 0, 0, 0, 0, wib)) {
		t.Fatalf("month bounds = %v .. %v, want 2027-01-01 .. 2027-02-01 (WIB)", repo.monthStart, repo.monthEnd)
	}
}

func TestDashboardChartFolding(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, wib)
	repo := &fakeDashboardRepository{counts: []domain.CategoryMonthCount{
		{Month: 1, Category: "Barcode", Count: 6},
		{Month: 1, Category: "Android", Count: 5},
		{Month: 3, Category: "Server", Count: 2},
		{Month: 3, Category: "Mobile Printer", Count: 9}, // not on the chart (yet)
		{Month: 13, Category: "Barcode", Count: 9},       // not a month
		{Month: 12, Category: "Barcode", Count: 9},       // after the current month of the current year
	}}
	got, err := newDashboard(repo, now).Overview(context.Background(), 2026)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	jan, mar, feb := got.Months[0], got.Months[2], got.Months[1]
	if jan.Barcode != 6 || jan.Android != 5 || jan.Server != 0 {
		t.Errorf("january = %+v, want 6/5/0", jan)
	}
	if mar.Server != 2 || mar.Barcode != 0 {
		t.Errorf("march = %+v, want server 2 only", mar)
	}
	if feb.Barcode+feb.Android+feb.Server != 0 {
		t.Errorf("february = %+v, want zeros", feb)
	}
	if len(got.Months) != 10 {
		t.Errorf("got %d months, want 10 (nothing after October)", len(got.Months))
	}
}

func TestDashboardPassesThroughAndFailsAsOne(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, wib)

	t.Run("stats and attention requests are passed through with the agreed limits", func(t *testing.T) {
		repo := &fakeDashboardRepository{
			stats:     domain.DashboardStats{RequestsThisMonth: 7, PendingApproval: 3, InProgress: 4, AssetsRegistered: 120},
			attention: []domain.AssetRequest{{ID: "REQ-1"}, {ID: "REQ-2"}},
		}
		got, err := newDashboard(repo, now).Overview(ctx, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Stats.AssetsRegistered != 120 || got.Stats.PendingApproval != 3 || len(got.Attention) != 2 {
			t.Fatalf("got %+v", got)
		}
		if repo.activityN != 5 || repo.attentionN != 5 {
			t.Fatalf("limits = %d activities, %d attention; want 5 and 5", repo.activityN, repo.attentionN)
		}
	})

	t.Run("a failing query fails the whole overview", func(t *testing.T) {
		boom := errors.New("database unavailable")
		if _, err := newDashboard(&fakeDashboardRepository{failWith: boom}, now).Overview(ctx, 0); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the repository error", err)
		}
	})
}

func TestDashboardActivities(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, wib)
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	repo := &fakeDashboardRepository{activity: []domain.ActivityRecord{
		{ID: "1", RequestID: "REQ-1", Actor: "Laras P.", Action: "Submitted", CreatedAt: ago(30 * time.Second)},
		{ID: "2", RequestID: "REQ-2", Actor: "Dimas W.", Action: "Approved", CreatedAt: ago(5 * time.Minute)},
		{ID: "3", RequestID: "REQ-3", Actor: "Dimas W.", Action: "Rejected", CreatedAt: ago(2 * time.Hour)},
		{ID: "4", RequestID: "REQ-4", Actor: "Bu Lita", Action: "Requested revision", CreatedAt: ago(26 * time.Hour)},
		{ID: "5", RequestID: "REQ-5", Actor: "EMP001", Action: "Data aset dicatat — Shipped", CreatedAt: ago(3 * 24 * time.Hour)},
		{ID: "6", RequestID: "REQ-6", Actor: "EMP001", Action: "Barang diterima — Delivered", CreatedAt: ago(59 * time.Minute)},
		{ID: "7", RequestID: "REQ-7", Actor: "", Action: "Fulfillment selesai — Completed", CreatedAt: ago(23 * time.Hour)},
		{ID: "8", RequestID: "REQ-8", Actor: "", Action: "Approved", CreatedAt: ago(time.Hour)},
		{ID: "9", RequestID: "REQ-9", Actor: "X", Action: "Something else", CreatedAt: ago(time.Hour)},
	}}
	got, err := newDashboard(repo, now).Overview(context.Background(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []domain.DashboardActivity{
		{ID: "1", Title: "New request REQ-1 from Laras P.", TimeAgo: "now", Type: "request"},
		{ID: "2", Title: "REQ-2 approved by Dimas W.", TimeAgo: "5m", Type: "approval"},
		{ID: "3", Title: "REQ-3 rejected by Dimas W.", TimeAgo: "2h", Type: "warning"},
		{ID: "4", Title: "REQ-4 revision requested by Bu Lita", TimeAgo: "1d", Type: "warning"},
		{ID: "5", Title: "REQ-5 shipped", TimeAgo: "3d", Type: "fulfillment"},
		{ID: "6", Title: "REQ-6 delivered", TimeAgo: "59m", Type: "fulfillment"},
		{ID: "7", Title: "REQ-7 fulfillment completed", TimeAgo: "23h", Type: "fulfillment"},
		{ID: "8", Title: "REQ-8 approved", TimeAgo: "1h", Type: "approval"},
		{ID: "9", Title: "REQ-9: Something else", TimeAgo: "1h", Type: "request"},
	}
	if len(got.Activities) != len(want) {
		t.Fatalf("got %d activities, want %d", len(got.Activities), len(want))
	}
	for i, w := range want {
		if got.Activities[i] != w {
			t.Errorf("activity %d = %+v, want %+v", i+1, got.Activities[i], w)
		}
	}
}

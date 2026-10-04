package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
)

// ErrInvalidDashboardYear: the requested year is outside the supported range.
var ErrInvalidDashboardYear = errors.New("year must be between 2000 and 2100")

const (
	dashboardMinYear = 2000
	dashboardMaxYear = 2100

	// The activity feed and the "needing attention" table both show five rows.
	dashboardActivityLimit  = 5
	dashboardAttentionLimit = 5
)

// businessZone is the time zone months and years are counted in (WIB, UTC+7),
// so a request made at 06:30 on the 1st does not count toward the previous month.
var businessZone = time.FixedZone("WIB", 7*3600)

type dashboardService struct {
	repo domain.DashboardRepository
	now  func() time.Time
}

// NewDashboardService creates a domain.DashboardService. now is the clock
// (time.Now in production), injected so month and year logic can be tested.
func NewDashboardService(repo domain.DashboardRepository, now func() time.Time) domain.DashboardService {
	return &dashboardService{repo: repo, now: now}
}

func (s *dashboardService) Overview(ctx context.Context, year int) (*domain.DashboardOverview, error) {
	now := s.now().In(businessZone)
	if year == 0 {
		year = now.Year()
	}
	if year < dashboardMinYear || year > dashboardMaxYear {
		return nil, ErrInvalidDashboardYear
	}

	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, businessZone)
	yearStart := time.Date(year, time.January, 1, 0, 0, 0, 0, businessZone)

	var (
		stats      domain.DashboardStats
		counts     []domain.CategoryMonthCount
		activity   []domain.ActivityRecord
		attention  []domain.AssetRequest
		errs       [4]error
		waitForAll sync.WaitGroup
	)
	// The four reads are independent; running them together costs one round
	// trip to the database instead of four.
	run := func(i int, fn func() error) {
		waitForAll.Add(1)
		go func() {
			defer waitForAll.Done()
			errs[i] = fn()
		}()
	}
	run(0, func() (err error) {
		stats, err = s.repo.Stats(ctx, monthStart, monthStart.AddDate(0, 1, 0))
		return err
	})
	run(1, func() (err error) {
		counts, err = s.repo.CategoryMonthCounts(ctx, yearStart, yearStart.AddDate(1, 0, 0))
		return err
	})
	run(2, func() (err error) {
		activity, err = s.repo.RecentActivity(ctx, dashboardActivityLimit)
		return err
	})
	run(3, func() (err error) {
		attention, err = s.repo.AttentionRequests(ctx, dashboardAttentionLimit)
		return err
	})
	waitForAll.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return &domain.DashboardOverview{
		Stats:      stats,
		Month:      now.Month(),
		Year:       year,
		Months:     foldMonths(counts, monthsToShow(year, now)),
		Activities: describeActivities(activity, now),
		Attention:  attention,
	}, nil
}

// monthsToShow is how many months of the year the chart covers: all twelve for
// a past year, up to the current month for this year, none for a future one.
func monthsToShow(year int, now time.Time) int {
	switch {
	case year < now.Year():
		return 12
	case year == now.Year():
		return int(now.Month())
	default:
		return 0
	}
}

// foldMonths turns (month, category, count) cells into one row per month.
// Categories the chart has no column for, and months outside 1..n, are ignored.
func foldMonths(counts []domain.CategoryMonthCount, n int) []domain.MonthlyRequestCount {
	months := make([]domain.MonthlyRequestCount, n)
	for i := range months {
		months[i].Month = i + 1
	}
	for _, c := range counts {
		if c.Month < 1 || c.Month > n {
			continue
		}
		m := &months[c.Month-1]
		switch c.Category {
		case "Barcode":
			m.Barcode += c.Count
		case "Android":
			m.Android += c.Count
		case "Server":
			m.Server += c.Count
		}
	}
	return months
}

func describeActivities(records []domain.ActivityRecord, now time.Time) []domain.DashboardActivity {
	out := make([]domain.DashboardActivity, 0, len(records))
	for _, r := range records {
		title, kind := describeActivity(r)
		out = append(out, domain.DashboardActivity{
			ID:      r.ID,
			Title:   title,
			TimeAgo: timeAgo(now.Sub(r.CreatedAt)),
			Type:    kind,
		})
	}
	return out
}

// describeActivity words a history event for the feed and picks its icon type.
func describeActivity(r domain.ActivityRecord) (title, kind string) {
	by := func(verb string) string {
		if r.Actor == "" {
			return fmt.Sprintf("%s %s", r.RequestID, verb)
		}
		return fmt.Sprintf("%s %s by %s", r.RequestID, verb, r.Actor)
	}

	switch {
	case r.Action == "Submitted":
		if r.Actor == "" {
			return "New request " + r.RequestID, "request"
		}
		return fmt.Sprintf("New request %s from %s", r.RequestID, r.Actor), "request"
	case r.Action == "Approved":
		return by("approved"), "approval"
	case r.Action == "Rejected":
		return by("rejected"), "warning"
	case r.Action == "Requested revision":
		return by("revision requested"), "warning"
	case strings.Contains(r.Action, "Shipped"):
		return r.RequestID + " shipped", "fulfillment"
	case strings.Contains(r.Action, "Delivered"):
		return r.RequestID + " delivered", "fulfillment"
	case strings.Contains(r.Action, "Completed"):
		return r.RequestID + " fulfillment completed", "fulfillment"
	default:
		return fmt.Sprintf("%s: %s", r.RequestID, r.Action), "request"
	}
}

// timeAgo formats an elapsed time the way the feed shows it: "now", "5m", "2h", "3d".
func timeAgo(elapsed time.Duration) string {
	switch {
	case elapsed < time.Minute:
		return "now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm", int(elapsed/time.Minute))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("%dh", int(elapsed/time.Hour))
	default:
		return fmt.Sprintf("%dd", int(elapsed/(24*time.Hour)))
	}
}

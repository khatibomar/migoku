package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// TestComputeStreakStats covers the pure streak arithmetic, including the rule
// that a streak stays alive while the last study day is today or yesterday.
func TestComputeStreakStats(t *testing.T) {
	ptr := func(v int) *int { return &v }

	tests := []struct {
		name         string
		days         []int // unsorted, as returned by the query
		today        int
		wantCurrent  int
		wantLongest  int
		wantTotal    int
		wantCurStart *int
		wantCurEnd   *int
	}{
		{
			name:        "empty",
			days:        nil,
			today:       100,
			wantCurrent: 0,
			wantLongest: 0,
			wantTotal:   0,
		},
		{
			name:         "current streak alive ending today",
			days:         []int{1, 2, 3, 4, 5},
			today:        5,
			wantCurrent:  5,
			wantLongest:  5,
			wantTotal:    5,
			wantCurStart: ptr(1),
			wantCurEnd:   ptr(5),
		},
		{
			name:         "current streak alive ending yesterday",
			days:         []int{10, 11, 12},
			today:        13,
			wantCurrent:  3,
			wantLongest:  3,
			wantTotal:    3,
			wantCurStart: ptr(10),
			wantCurEnd:   ptr(12),
		},
		{
			name:         "current streak broken when last day older than yesterday",
			days:         []int{10, 11, 12},
			today:        20,
			wantCurrent:  0,
			wantLongest:  3,
			wantTotal:    3,
			wantCurStart: ptr(10),
			wantCurEnd:   ptr(12),
		},
		{
			name:         "longest earlier run beats current run",
			days:         []int{1, 2, 3, 4, 5, 6, 7, 50, 51},
			today:        51,
			wantCurrent:  2,
			wantLongest:  7,
			wantTotal:    9,
			wantCurStart: ptr(50),
			wantCurEnd:   ptr(51),
		},
		{
			name:         "unsorted input",
			days:         []int{5, 1, 3, 2, 4},
			today:        5,
			wantCurrent:  5,
			wantLongest:  5,
			wantTotal:    5,
			wantCurStart: ptr(1),
			wantCurEnd:   ptr(5),
		},
		{
			name:         "single day is alive today",
			days:         []int{42},
			today:        42,
			wantCurrent:  1,
			wantLongest:  1,
			wantTotal:    1,
			wantCurStart: ptr(42),
			wantCurEnd:   ptr(42),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeStreakStats(tt.days, tt.today)
			if got.CurrentStreakDays != tt.wantCurrent {
				t.Errorf("CurrentStreakDays = %d, want %d", got.CurrentStreakDays, tt.wantCurrent)
			}
			if got.LongestStreakDays != tt.wantLongest {
				t.Errorf("LongestStreakDays = %d, want %d", got.LongestStreakDays, tt.wantLongest)
			}
			if got.TotalStudyDays != tt.wantTotal {
				t.Errorf("TotalStudyDays = %d, want %d", got.TotalStudyDays, tt.wantTotal)
			}
			assertOptionalInt(t, "CurrentStreakStart", got.CurrentStreakStart, tt.wantCurStart)
			assertOptionalInt(t, "CurrentStreakEnd", got.CurrentStreakEnd, tt.wantCurEnd)
		})
	}
}

func assertOptionalInt(t *testing.T, name string, got, want *int) {
	t.Helper()
	switch {
	case got == nil && want == nil:
		return
	case got == nil || want == nil:
		t.Errorf("%s = %v, want %v", name, got, want)
	case *got != *want:
		t.Errorf("%s = %d, want %d", name, *got, *want)
	}
}

// buildStreakClient builds a client backed by a synthetic SRS db containing the
// given review days for the "ja" language.
func buildStreakClient(t *testing.T, activeDay string, reviewDays []int) *MigakuClient {
	t.Helper()

	path := filepath.Join(t.TempDir(), "srs.db")
	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schema := `
CREATE TABLE keyValue (key TEXT PRIMARY KEY, entry TEXT);
CREATE TABLE card_type (id INTEGER PRIMARY KEY, lang TEXT);
CREATE TABLE card (id INTEGER PRIMARY KEY, cardTypeId INTEGER, del INTEGER DEFAULT 0);
CREATE TABLE review (id INTEGER PRIMARY KEY, cardId INTEGER, day INTEGER, del INTEGER DEFAULT 0);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO card_type (id, lang) VALUES (1, 'ja')`); err != nil {
		t.Fatalf("insert card_type: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO card (id, cardTypeId, del) VALUES (1, 1, 0)`); err != nil {
		t.Fatalf("insert card: %v", err)
	}
	if activeDay != "" {
		if _, err := db.Exec(
			`INSERT INTO keyValue (key, entry) VALUES ('study.activeDay.currentDate', ?)`,
			activeDay,
		); err != nil {
			t.Fatalf("insert keyValue: %v", err)
		}
	}
	for i, day := range reviewDays {
		if _, err := db.Exec(
			`INSERT INTO review (id, cardId, day, del) VALUES (?, 1, ?, 0)`,
			i+1, day,
		); err != nil {
			t.Fatalf("insert review: %v", err)
		}
	}

	return &MigakuClient{
		db:     db,
		dbPath: path,
		key:    "test",
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// TestGetStreakIntegration exercises GetStreak end to end against a synthetic
// database, mirroring the real-world shape: a long run, a gap, then a short
// current streak.
func TestGetStreakIntegration(t *testing.T) {
	today := time.Now()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	todayNum := dayNumber(today)

	// 30-day run long ago, then a gap, then the last 4 days ending today.
	days := make([]int, 0, 34)
	for i := 0; i < 30; i++ {
		days = append(days, todayNum-100+i)
	}
	for i := 3; i >= 0; i-- {
		days = append(days, todayNum-i)
	}

	client := buildStreakClient(t, today.Format("2006-01-02"), days)

	stats, err := testService().GetStreak(context.Background(), client, "ja", "")
	if err != nil {
		t.Fatalf("GetStreak: %v", err)
	}

	if stats.CurrentStreakDays != 4 {
		t.Errorf("CurrentStreakDays = %d, want 4", stats.CurrentStreakDays)
	}
	if stats.LongestStreakDays != 30 {
		t.Errorf("LongestStreakDays = %d, want 30", stats.LongestStreakDays)
	}
	if stats.TotalStudyDays != 34 {
		t.Errorf("TotalStudyDays = %d, want 34", stats.TotalStudyDays)
	}
	if stats.CurrentStreakStart == nil || *stats.CurrentStreakStart != todayNum-3 {
		t.Errorf("CurrentStreakStart = %v, want %d", stats.CurrentStreakStart, todayNum-3)
	}
	if stats.CurrentStreakEnd == nil || *stats.CurrentStreakEnd != todayNum {
		t.Errorf("CurrentStreakEnd = %v, want %d", stats.CurrentStreakEnd, todayNum)
	}
	if stats.LastStudyDay == nil || *stats.LastStudyDay != todayNum {
		t.Errorf("LastStudyDay = %v, want %d", stats.LastStudyDay, todayNum)
	}
}

// TestGetStreakEmpty ensures a language with no reviews returns zeroes rather
// than an error (the class of NULL-scan bug fixed for /stats/study).
func TestGetStreakEmpty(t *testing.T) {
	client := buildStreakClient(t, "", nil)

	stats, err := testService().GetStreak(context.Background(), client, "ja", "")
	if err != nil {
		t.Fatalf("GetStreak: %v", err)
	}
	if stats.CurrentStreakDays != 0 || stats.LongestStreakDays != 0 || stats.TotalStudyDays != 0 {
		t.Errorf("expected zeroed stats, got %+v", stats)
	}
	if stats.LastStudyDay != nil {
		t.Errorf("LastStudyDay = %v, want nil", stats.LastStudyDay)
	}
}

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

// buildTestClient creates a MigakuClient backed by a synthetic SRS database
// so the schedule logic can be exercised without a live Migaku account.
func buildTestClient(t *testing.T, activeDay string, cards []testCard) *MigakuClient {
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
CREATE TABLE card (
  id INTEGER PRIMARY KEY,
  cardTypeId INTEGER,
  deckId INTEGER,
  due INTEGER,
  interval INTEGER,
  reviewCount INTEGER DEFAULT 1,
  del INTEGER DEFAULT 0
);
CREATE TABLE deck (
  id INTEGER PRIMARY KEY,
  lang TEXT,
  newBatchMax INTEGER DEFAULT 0,
  del INTEGER DEFAULT 0
);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO card_type (id, lang) VALUES (1, 'ja')`); err != nil {
		t.Fatalf("insert card_type: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO deck (id, lang, newBatchMax, del) VALUES (1, 'ja', 0, 0)`); err != nil {
		t.Fatalf("insert deck: %v", err)
	}
	if activeDay != "" {
		if _, err := db.Exec(
			`INSERT INTO keyValue (key, entry) VALUES ('study.activeDay.currentDate', ?)`,
			activeDay,
		); err != nil {
			t.Fatalf("insert keyValue: %v", err)
		}
	}

	for i, c := range cards {
		interval := c.interval
		if interval == 0 {
			interval = 1
		}
		if _, err := db.Exec(
			`INSERT INTO card (id, cardTypeId, deckId, due, interval, reviewCount, del) VALUES (?, 1, 1, ?, ?, ?, 0)`,
			i+1, c.due, interval, c.reviewCount,
		); err != nil {
			t.Fatalf("insert card: %v", err)
		}
	}

	return &MigakuClient{
		db:     db,
		dbPath: path,
		key:    "test",
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

type testCard struct {
	due         int
	interval    int
	reviewCount int
}

func testService() *MigakuService {
	return NewMigakuService(NewRepository(), NewCache(time.Minute))
}

// TestScheduleCountsOverdueAsDueNow is the regression test for the widget
// reporting "nothing due today" for someone who has not studied yet.
func TestScheduleCountsOverdueAsDueNow(t *testing.T) {
	today := time.Now()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	todayNum := dayNumber(today)

	yesterday := today.AddDate(0, 0, -1)

	client := buildTestClient(t, yesterday.Format("2006-01-02"), []testCard{
		{due: todayNum, interval: 1, reviewCount: 1},
		{due: todayNum, interval: 1, reviewCount: 1},
		{due: todayNum - 1, interval: 1, reviewCount: 1},
		{due: todayNum + 3, interval: 1, reviewCount: 1},
	})

	schedule, err := testService().GetSchedule(context.Background(), client, "ja", "", 7)
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}

	if schedule.DueToday != 2 {
		t.Errorf("due_today = %d, want 2", schedule.DueToday)
	}
	if schedule.Overdue != 1 {
		t.Errorf("overdue = %d, want 1", schedule.Overdue)
	}
	if schedule.DueNow != 3 {
		t.Errorf("due_now = %d, want 3 (2 due today + 1 overdue)", schedule.DueNow)
	}
	if schedule.Reviews != 3 {
		t.Errorf("reviews = %d, want 3", schedule.Reviews)
	}
	if schedule.Today != today.Format("2006-01-02") {
		t.Errorf("today = %q, want %q", schedule.Today, today.Format("2006-01-02"))
	}
}

// TestScheduleIgnoresStaleActiveDay checks the schedule does not depend on
// the SRS active day at all: two snapshots that differ only in that key must
// produce the same schedule.
func TestScheduleIgnoresStaleActiveDay(t *testing.T) {
	today := time.Now()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	todayNum := dayNumber(today)

	cards := []testCard{{due: todayNum, interval: 1, reviewCount: 1}, {due: todayNum + 1, interval: 1, reviewCount: 1}}

	fresh := buildTestClient(t, today.Format("2006-01-02"), cards)
	stale := buildTestClient(t, today.AddDate(0, 0, -5).Format("2006-01-02"), cards)

	freshSchedule, err := testService().GetSchedule(context.Background(), fresh, "ja", "", 7)
	if err != nil {
		t.Fatalf("GetSchedule(fresh): %v", err)
	}
	staleSchedule, err := testService().GetSchedule(context.Background(), stale, "ja", "", 7)
	if err != nil {
		t.Fatalf("GetSchedule(stale): %v", err)
	}

	if freshSchedule.DueToday != staleSchedule.DueToday {
		t.Errorf(
			"due_today depends on the active day: fresh = %d, stale = %d",
			freshSchedule.DueToday, staleSchedule.DueToday,
		)
	}
	if freshSchedule.DueNow != staleSchedule.DueNow {
		t.Errorf(
			"due_now depends on the active day: fresh = %d, stale = %d",
			freshSchedule.DueNow, staleSchedule.DueNow,
		)
	}
}

// TestScheduleWithoutActiveDayKey covers a database that has no
// 'study.activeDay.currentDate' row at all.
func TestScheduleWithoutActiveDayKey(t *testing.T) {
	today := time.Now()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	todayNum := dayNumber(today)

	client := buildTestClient(t, "", []testCard{{due: todayNum, interval: 1, reviewCount: 1}})

	schedule, err := testService().GetSchedule(context.Background(), client, "ja", "", 7)
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}
	if schedule.DueToday != 1 {
		t.Errorf("due_today = %d, want 1", schedule.DueToday)
	}
}

// TestScheduleDaysAreConsecutive checks the per-day breakdown lines up with
// the dates it reports.
func TestScheduleDaysAreConsecutive(t *testing.T) {
	today := time.Now()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	todayNum := dayNumber(today)

	client := buildTestClient(t, today.Format("2006-01-02"), []testCard{
		{due: todayNum, interval: 1, reviewCount: 1},
		{due: todayNum + 1, interval: 1, reviewCount: 1},
		{due: todayNum + 1, interval: 1, reviewCount: 1},
	})

	schedule, err := testService().GetSchedule(context.Background(), client, "ja", "", 7)
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}

	if len(schedule.Days) != 7 {
		t.Fatalf("len(days) = %d, want 7", len(schedule.Days))
	}
	if schedule.Days[0].Date != today.Format("2006-01-02") {
		t.Errorf("days[0].date = %q, want %q", schedule.Days[0].Date, today.Format("2006-01-02"))
	}
	if schedule.Days[0].DueCount != 1 {
		t.Errorf("days[0].due_count = %d, want 1", schedule.Days[0].DueCount)
	}
	if schedule.Days[1].DueCount != 2 {
		t.Errorf("days[1].due_count = %d, want 2", schedule.Days[1].DueCount)
	}
	if schedule.Days[2].DueCount != 0 {
		t.Errorf("days[2].due_count = %d, want 0", schedule.Days[2].DueCount)
	}
}

// TestScheduleSplitsLearningAndKnown checks the learning/known split, which
// the widget uses for the interval threshold.
func TestScheduleSplitsLearningAndKnown(t *testing.T) {
	today := time.Now()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	todayNum := dayNumber(today)

	client := buildTestClient(t, today.Format("2006-01-02"), []testCard{
		{due: todayNum, interval: 1, reviewCount: 1},   // learning
		{due: todayNum, interval: 25, reviewCount: 1},  // known
		{due: todayNum, interval: 100, reviewCount: 1}, // known
	})

	schedule, err := testService().GetSchedule(context.Background(), client, "ja", "", 7)
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}
	if schedule.LearningToday != 1 {
		t.Errorf("learning_today = %d, want 1", schedule.LearningToday)
	}
	if schedule.KnownToday != 2 {
		t.Errorf("known_today = %d, want 2", schedule.KnownToday)
	}
}

// TestScheduleRequiresLang checks the endpoint rejects a missing language.
func TestScheduleRequiresLang(t *testing.T) {
	client := buildTestClient(t, "", nil)
	_, err := testService().GetSchedule(context.Background(), client, "", "", 7)
	if err == nil {
		t.Fatal("expected an error for an empty lang")
	}
}

// TestScheduleSeparatesNewCardsFromReviews checks that never-studied cards are
// reported as new_cards instead of being counted as overdue.
func TestScheduleSeparatesNewCardsFromReviews(t *testing.T) {
	today := time.Now()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	todayNum := dayNumber(today)

	client := buildTestClient(t, today.Format("2006-01-02"), []testCard{
		{due: todayNum, interval: 1, reviewCount: 4},     // review, due today
		{due: todayNum - 1, interval: 1, reviewCount: 2}, // review, overdue
		{due: 0, interval: 0, reviewCount: 0},            // never studied
		{due: 0, interval: 0, reviewCount: 0},            // never studied
		{due: 0, interval: 0, reviewCount: 0},            // never studied
	})
	setDeckNewBatchMax(t, client, 1, 12)

	schedule, err := testService().GetSchedule(context.Background(), client, "ja", "", 7)
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}

	if schedule.Overdue != 1 {
		t.Errorf("overdue = %d, want 1 (new cards must not be overdue)", schedule.Overdue)
	}
	if schedule.Reviews != 2 {
		t.Errorf("reviews = %d, want 2 (1 due today + 1 overdue)", schedule.Reviews)
	}
	if schedule.NewCards != 3 {
		t.Errorf("new_cards = %d, want 3", schedule.NewCards)
	}
	if schedule.Days[0].DueCount != 1 {
		t.Errorf("days[0].due_count = %d, want 1 (new cards excluded)", schedule.Days[0].DueCount)
	}
}

// TestScheduleCapsNewCardsByDeckBatch checks the new-card count never exceeds
// what the decks allow per session.
func TestScheduleCapsNewCardsByDeckBatch(t *testing.T) {
	today := time.Now()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())

	cards := make([]testCard, 0, 30)
	for range 30 {
		cards = append(cards, testCard{due: 0, interval: 0, reviewCount: 0})
	}

	client := buildTestClient(t, today.Format("2006-01-02"), cards)
	setDeckNewBatchMax(t, client, 1, 12)

	schedule, err := testService().GetSchedule(context.Background(), client, "ja", "", 7)
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}
	if schedule.NewCards != 12 {
		t.Errorf("new_cards = %d, want 12 (120 waiting, batch cap 12)", schedule.NewCards)
	}
}

// TestScheduleReviewsExcludeNewCards pins the split the widget notification
// shows: reviews are the studied cards due now, new cards are the queue.
func TestScheduleReviewsExcludeNewCards(t *testing.T) {
	today := time.Now()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	todayNum := dayNumber(today)

	client := buildTestClient(t, today.Format("2006-01-02"), []testCard{
		{due: todayNum - 1, interval: 3, reviewCount: 1}, // yesterday's backlog
		{due: 0, interval: 0, reviewCount: 0},            // new
	})
	setDeckNewBatchMax(t, client, 1, 12)

	schedule, err := testService().GetSchedule(context.Background(), client, "ja", "", 7)
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}

	if schedule.Overdue != 1 || schedule.Reviews != 1 {
		t.Errorf("overdue/reviews = %d/%d, want 1/1", schedule.Overdue, schedule.Reviews)
	}
	if schedule.NewCards != 1 {
		t.Errorf("new_cards = %d, want 1", schedule.NewCards)
	}
}

// TestScheduleBacklogIsLastSRSDayOnly checks that only the previous SRS day
// counts as the review backlog.
func TestScheduleBacklogIsLastSRSDayOnly(t *testing.T) {
	today := time.Now()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	todayNum := dayNumber(today)

	client := buildTestClient(t, today.Format("2006-01-02"), []testCard{
		{due: todayNum, interval: 1, reviewCount: 3},       // due today
		{due: todayNum - 1, interval: 1, reviewCount: 2},   // yesterday's backlog
		{due: todayNum - 1, interval: 1, reviewCount: 2},   // yesterday's backlog
		{due: todayNum - 400, interval: 1, reviewCount: 1}, // abandoned months ago
	})
	setDeckNewBatchMax(t, client, 1, 12)

	schedule, err := testService().GetSchedule(context.Background(), client, "ja", "", 7)
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}

	if schedule.DueToday != 1 {
		t.Errorf("due_today = %d, want 1", schedule.DueToday)
	}
	if schedule.Overdue != 2 {
		t.Errorf("overdue = %d, want 2 (only yesterday, not the abandoned card)", schedule.Overdue)
	}
	if schedule.Reviews != 3 {
		t.Errorf("reviews = %d, want 3", schedule.Reviews)
	}
}

// TestScheduleCapsNewCardsByActiveDecks checks the new-card allowance only sums
// decks that actually hold cards.
func TestScheduleCapsNewCardsByActiveDecks(t *testing.T) {
	today := time.Now()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())

	client := buildTestClient(t, today.Format("2006-01-02"), []testCard{
		{due: 0, interval: 0, reviewCount: 0},
		{due: 0, interval: 0, reviewCount: 0},
	})
	setDeckNewBatchMax(t, client, 1, 12)

	if _, err := client.db.Exec(
		`INSERT INTO deck (id, lang, newBatchMax, del) VALUES (2, 'ja', 10, 0), (3, 'ja', 12, 0)`,
	); err != nil {
		t.Fatalf("insert placeholder decks: %v", err)
	}

	schedule, err := testService().GetSchedule(context.Background(), client, "ja", "", 7)
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}

	if schedule.NewCards != 2 {
		t.Errorf("new_cards = %d, want 2 (only the deck holding cards)", schedule.NewCards)
	}
}

// setDeckNewBatchMax points the test deck at a per-session new-card cap.
func setDeckNewBatchMax(t *testing.T, client *MigakuClient, deckID, newBatchMax int) {
	t.Helper()
	if _, err := client.db.Exec(
		`UPDATE deck SET newBatchMax = ? WHERE id = ?`,
		newBatchMax, deckID,
	); err != nil {
		t.Fatalf("set newBatchMax: %v", err)
	}
}

package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Word represents a word in the domain
type Word struct {
	DictForm    string `json:"dictForm"`
	Secondary   string `json:"secondary"`
	KnownStatus string `json:"knownStatus,omitempty"`
}

const (
	statusKnown    = "known"
	statusLearning = "learning"
	statusUnknown  = "unknown"
	statusIgnored  = "ignored"

	cacheAllKey = "all"

	periodAllTime = "All time"
)

// WordFromRow creates a Word from a repository wordRow
func WordFromRow(row wordRow) Word {
	return Word(row)
}

// WordsFromRows creates a slice of Words from repository wordRows
func WordsFromRows(rows []wordRow) []Word {
	words := make([]Word, len(rows))
	for i, row := range rows {
		words[i] = WordFromRow(row)
	}
	return words
}

// Deck represents a deck in the domain
type Deck struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// DeckFromRow creates a Deck from a repository deckRow
func DeckFromRow(row deckRow) Deck {
	return Deck(row)
}

// DecksFromRows creates a slice of Decks from repository deckRows
func DecksFromRows(rows []deckRow) []Deck {
	decks := make([]Deck, len(rows))
	for i, row := range rows {
		decks[i] = DeckFromRow(row)
	}
	return decks
}

// StatusCounts represents aggregated status counts
type StatusCounts struct {
	KnownCount    int `json:"known_count"`
	LearningCount int `json:"learning_count"`
	UnknownCount  int `json:"unknown_count"`
	IgnoredCount  int `json:"ignored_count"`
}

// StatusCountsFromRows creates StatusCounts from repository statusCountRows
func StatusCountsFromRows(rows []statusCountRow) StatusCounts {
	counts := StatusCounts{}
	for _, row := range rows {
		switch row.Status {
		case "KNOWN":
			counts.KnownCount = row.Count
		case "LEARNING":
			counts.LearningCount = row.Count
		case "UNKNOWN":
			counts.UnknownCount = row.Count
		case "IGNORED":
			counts.IgnoredCount = row.Count
		}
	}
	return counts
}

// Table represents a database table
type Table struct {
	Name string `json:"name"`
}

// TableFromRow creates a Table from a repository tableRow
func TableFromRow(row tableRow) Table {
	return Table(row)
}

// TablesFromRows creates a slice of Tables from repository tableRows
func TablesFromRows(rows []tableRow) []Table {
	tables := make([]Table, len(rows))
	for i, row := range rows {
		tables[i] = TableFromRow(row)
	}
	return tables
}

// MigakuService handles business logic and caching for Migaku data
type MigakuService struct {
	repo  *Repository
	cache *Cache
}

func (s *MigakuService) scopedCacheKey(client *MigakuClient, key string) string {
	if client == nil || client.key == "" {
		return key
	}
	return "client:" + client.key + ":" + key
}

// NewMigakuService creates a new service instance
func NewMigakuService(repo *Repository, cache *Cache) *MigakuService {
	return &MigakuService{
		repo:  repo,
		cache: cache,
	}
}

// GetWords retrieves words with optional status and language filters
func (s *MigakuService) GetWords(ctx context.Context, client *MigakuClient, lang, status string) ([]Word, error) {
	if status != "" && status != statusKnown && status != statusLearning && status != statusUnknown && status != statusIgnored {
		return nil, errors.New("invalid status: must be one of: known, learning, unknown, ignored")
	}

	cacheKey := "words:"
	if status == "" {
		cacheKey += "all:"
	} else {
		cacheKey += status + ":"
	}
	if lang == "" {
		cacheKey += cacheAllKey
	} else {
		cacheKey += lang
	}
	cacheKey = s.scopedCacheKey(client, cacheKey)

	if cached, ok := s.cache.Get(cacheKey); ok {
		if words, ok := cached.([]Word); ok {
			return words, nil
		}
	}

	var dbStatus string
	if status != "" {
		switch status {
		case statusKnown:
			dbStatus = "KNOWN"
		case statusLearning:
			dbStatus = "LEARNING"
		case statusUnknown:
			dbStatus = "UNKNOWN"
		case statusIgnored:
			dbStatus = "IGNORED"
		}
	}

	limit := 0
	if dbStatus == "" {
		limit = 10000
	}

	rows, err := s.repo.GetWords(ctx, client, lang, dbStatus, limit)
	if err != nil {
		return nil, err
	}

	words := WordsFromRows(rows)
	s.cache.Set(cacheKey, words)

	return words, nil
}

// GetDecks retrieves all decks with caching
func (s *MigakuService) GetDecks(ctx context.Context, client *MigakuClient) ([]Deck, error) {
	cacheKey := s.scopedCacheKey(client, "decks")

	if cached, ok := s.cache.Get(cacheKey); ok {
		if decks, ok := cached.([]Deck); ok {
			return decks, nil
		}
	}

	rows, err := s.repo.GetDecks(ctx, client)
	if err != nil {
		return nil, err
	}

	decks := DecksFromRows(rows)
	s.cache.Set(cacheKey, decks)

	return decks, nil
}

// GetStatusCounts retrieves status counts with caching
func (s *MigakuService) GetStatusCounts(ctx context.Context, client *MigakuClient, lang, deckID string) (*StatusCounts, error) {
	cacheKey := s.scopedCacheKey(client, s.buildStatusCountsCacheKey(lang, deckID))

	if cached, ok := s.cache.Get(cacheKey); ok {
		if counts, ok := cached.(*StatusCounts); ok {
			return counts, nil
		}
	}

	rows, err := s.repo.GetStatusCounts(ctx, client, lang, deckID)
	if err != nil {
		return nil, err
	}

	counts := StatusCountsFromRows(rows)
	s.cache.Set(cacheKey, &counts)

	return &counts, nil
}

// GetTables retrieves all database tables with caching
func (s *MigakuService) GetTables(ctx context.Context, client *MigakuClient) ([]Table, error) {
	cacheKey := s.scopedCacheKey(client, "tables")

	if cached, ok := s.cache.Get(cacheKey); ok {
		if tables, ok := cached.([]Table); ok {
			return tables, nil
		}
	}

	rows, err := s.repo.GetTables(ctx, client)
	if err != nil {
		return nil, err
	}

	tables := TablesFromRows(rows)
	s.cache.Set(cacheKey, tables)

	return tables, nil
}

// buildStatusCountsCacheKey builds a cache key for status counts
func (s *MigakuService) buildStatusCountsCacheKey(lang, deckID string) string {
	cacheKey := "status:counts:"

	if deckID == "" {
		cacheKey += cacheAllKey + ":"
	} else {
		cacheKey += fmt.Sprintf("deck:%s:", deckID)
	}

	if lang == "" {
		cacheKey += cacheAllKey
	} else {
		cacheKey += lang
	}

	return cacheKey
}

// DifficultWord represents words with high fail rates
type DifficultWord struct {
	DictForm      string  `json:"dictForm"`
	Secondary     string  `json:"secondary"`
	PartOfSpeech  string  `json:"partOfSpeech"`
	KnownStatus   string  `json:"knownStatus"`
	TotalReviews  int     `json:"total_reviews"`
	FailedReviews int     `json:"failed_reviews"`
	FailRate      float64 `json:"fail_rate"`
}

// GetDifficultWords retrieves words with highest fail rates
func (s *MigakuService) GetDifficultWords(
	ctx context.Context,
	client *MigakuClient,
	lang string,
	limit int,
	deckID string,
) ([]DifficultWord, error) {
	if limit == 0 {
		limit = 50
	}
	cacheKey := s.scopedCacheKey(client, fmt.Sprintf("difficult:words:%s:%d:%s", lang, limit, deckID))

	if cached, ok := s.cache.Get(cacheKey); ok {
		if words, ok := cached.([]DifficultWord); ok {
			return words, nil
		}
	}

	rows, err := s.repo.GetDifficultWords(ctx, client, lang, limit, deckID)
	if err != nil {
		return nil, err
	}

	words := make([]DifficultWord, len(rows))
	for i, row := range rows {
		words[i] = DifficultWord(row)
	}

	s.cache.Set(cacheKey, words)
	return words, nil
}

// FieldMetadata represents metadata about a database column
type FieldMetadata struct {
	Type       string `json:"type"`
	NotNull    bool   `json:"notNull"`
	PrimaryKey bool   `json:"primaryKey"`
}

// DatabaseSchema represents the database schema structure
type DatabaseSchema map[string]map[string]FieldMetadata

// GetDatabaseSchema retrieves the database schema and transforms it into a nested structure
func (s *MigakuService) GetDatabaseSchema(ctx context.Context, client *MigakuClient) (DatabaseSchema, error) {
	cacheKey := s.scopedCacheKey(client, "database:schema")

	if cached, ok := s.cache.Get(cacheKey); ok {
		if schema, ok := cached.(DatabaseSchema); ok {
			return schema, nil
		}
	}

	rows, err := s.repo.GetDatabaseSchema(ctx, client)
	if err != nil {
		return nil, err
	}

	tableToFields := make(DatabaseSchema)

	for _, row := range rows {
		if _, exists := tableToFields[row.TableName]; !exists {
			tableToFields[row.TableName] = make(map[string]FieldMetadata)
		}

		tableToFields[row.TableName][row.ColumnName] = FieldMetadata{
			Type:       row.ColumnType,
			NotNull:    row.IsNotNull != 0,
			PrimaryKey: row.IsPrimaryKey != 0,
		}
	}

	s.cache.Set(cacheKey, tableToFields)
	return tableToFields, nil
}

type WordStats struct {
	KnownCount    int `json:"known_count"`
	LearningCount int `json:"learning_count"`
	UnknownCount  int `json:"unknown_count"`
	IgnoredCount  int `json:"ignored_count"`
}

type DueStats struct {
	Labels         []string `json:"labels"`
	Counts         []int    `json:"counts"`
	KnownCounts    []int    `json:"knownCounts"`
	LearningCounts []int    `json:"learningCounts"`
}

type IntervalStats struct {
	Labels []string `json:"labels"`
	Counts []int    `json:"counts"`
}

type StudyStats struct {
	DaysStudied              int     `json:"days_studied"`
	DaysStudiedPercent       int     `json:"days_studied_percent"`
	TotalReviews             int     `json:"total_reviews"`
	AvgReviewsPerCalendarDay float64 `json:"avg_reviews_per_calendar_day"`
	PeriodDays               int     `json:"period_days"`
	PassRate                 int     `json:"pass_rate"`
	NewCardsPerDay           float64 `json:"new_cards_per_day"`
	TotalNewCards            int     `json:"total_new_cards"`
	TotalCardsAdded          int     `json:"total_cards_added"`
	CardsAddedPerDay         float64 `json:"cards_added_per_day"`
	TotalCardsLearned        int     `json:"total_cards_learned"`
	CardsLearnedPerDay       float64 `json:"cards_learned_per_day"`
	TotalTimeNewCardsSeconds int     `json:"total_time_new_cards_seconds"`
	AvgTimeNewCardSeconds    float64 `json:"avg_time_new_card_seconds"`
	TotalTimeReviewsSeconds  int     `json:"total_time_reviews_seconds"`
	AvgTimeReviewSeconds     float64 `json:"avg_time_review_seconds"`
}

const msPerDay = int64(24 * 60 * 60 * 1000)

func (s *MigakuService) GetWordStats(ctx context.Context, client *MigakuClient, lang, deckID string) (*WordStats, error) {
	if lang == "" {
		return nil, errors.New("lang parameter is required")
	}

	useDeckFilter := deckID != "" && deckID != cacheAllKey

	query := `
  SELECT
      SUM(CASE WHEN knownStatus = 'KNOWN' THEN 1 ELSE 0 END) as known_count,
      SUM(CASE WHEN knownStatus = 'LEARNING' THEN 1 ELSE 0 END) as learning_count,
      SUM(CASE WHEN knownStatus = 'UNKNOWN' THEN 1 ELSE 0 END) as unknown_count,
      SUM(CASE WHEN knownStatus = 'IGNORED' THEN 1 ELSE 0 END) as ignored_count
  FROM WordList
  WHERE language = ? AND del = 0`

	params := []any{lang}

	if useDeckFilter {
		query = `
  SELECT
    SUM(CASE WHEN w.knownStatus = 'KNOWN' THEN 1 ELSE 0 END) as known_count,
    SUM(CASE WHEN w.knownStatus = 'LEARNING' THEN 1 ELSE 0 END) as learning_count,
    SUM(CASE WHEN w.knownStatus = 'UNKNOWN' THEN 1 ELSE 0 END) as unknown_count,
    SUM(CASE WHEN w.knownStatus = 'IGNORED' THEN 1 ELSE 0 END) as ignored_count
  FROM (
    SELECT DISTINCT w.dictForm, w.knownStatus
    FROM WordList w
    JOIN CardWordRelation cwr ON w.dictForm = cwr.dictForm
    JOIN card c ON cwr.cardId = c.id
    JOIN deck d ON c.deckId = d.id
    WHERE w.language = ? AND w.del = 0 AND d.id = ? AND c.del = 0
  ) as w`
		params = []any{lang, deckID}
	}

	cacheKey := s.scopedCacheKey(client, fmt.Sprintf("stats:words:%s:%s", lang, deckID))
	if cached, ok := s.cache.Get(cacheKey); ok {
		if ws, ok := cached.(*WordStats); ok {
			return ws, nil
		}
	}

	type wordStatsRow struct {
		KnownCount    int `db:"known_count"    json:"known_count"`
		LearningCount int `db:"learning_count" json:"learning_count"`
		UnknownCount  int `db:"unknown_count"  json:"unknown_count"`
		IgnoredCount  int `db:"ignored_count"  json:"ignored_count"`
	}

	rows, err := runQuery[wordStatsRow](ctx, client, query, params...)
	if err != nil {
		return nil, err
	}

	stats := &WordStats{}
	if len(rows) > 0 {
		row := rows[0]
		stats.KnownCount = row.KnownCount
		stats.LearningCount = row.LearningCount
		stats.UnknownCount = row.UnknownCount
		stats.IgnoredCount = row.IgnoredCount
	}

	s.cache.Set(cacheKey, stats)
	return stats, nil
}

func (s *MigakuService) GetDueStats(ctx context.Context, client *MigakuClient, lang, deckID, periodID string) (*DueStats, error) {
	if lang == "" {
		return nil, errors.New("lang parameter is required")
	}

	if periodID == "" {
		periodID = "1 Month"
	}

	cacheKey := s.scopedCacheKey(client, fmt.Sprintf("stats:due:%s:%s:%s", lang, deckID, periodID))
	if cached, ok := s.cache.Get(cacheKey); ok {
		if ds, ok := cached.(*DueStats); ok {
			return ds, nil
		}
	}

	currentDate := time.Now()
	currentDate = time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 0, 0, 0, 0, currentDate.Location())

	type currentDateRow struct {
		Entry string `db:"entry" json:"entry"`
	}

	dateRows, err := runQuery[currentDateRow](ctx, client, `
SELECT entry 
FROM keyValue
WHERE key = 'study.activeDay.currentDate';`)
	if err == nil && len(dateRows) > 0 && dateRows[0].Entry != "" {
		if parsed, parseErr := time.Parse("2006-01-02", dateRows[0].Entry); parseErr == nil {
			currentDate = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, currentDate.Location())
		}
	}

	chartStartDate := time.Date(2020, time.January, 1, 0, 0, 0, 0, currentDate.Location())
	currentDelta := currentDate.UnixMilli() - chartStartDate.UnixMilli()
	currentDayNumber := int(currentDelta / msPerDay)

	var forecastDays int
	var endDayNumber int

	switch periodID {
	case periodAllTime:
		forecastDays = 3650

		type maxDueRow struct {
			MaxDue *int `db:"maxDue" json:"maxDue"`
		}

		maxDueQuery := `
SELECT MAX(due) as maxDue
FROM card c
JOIN card_type ct ON c.cardTypeId = ct.id
WHERE ct.lang = ? AND c.due >= ? AND c.del = 0`
		maxDueParams := []any{lang, currentDayNumber}
		useDeckFilter := deckID != "" && deckID != cacheAllKey
		if useDeckFilter {
			maxDueQuery += deckIDClause
			maxDueParams = append(maxDueParams, deckID)
		}

		maxDueRows, err := runQuery[maxDueRow](ctx, client, maxDueQuery, maxDueParams...)
		if err == nil && len(maxDueRows) > 0 && maxDueRows[0].MaxDue != nil {
			endDayNumber = *maxDueRows[0].MaxDue
		} else {
			endDayNumber = currentDayNumber + forecastDays - 1
		}
	case "1 Year":
		endDate := currentDate.AddDate(1, 0, 0)
		diff := float64(endDate.UnixMilli()-currentDate.UnixMilli()) / float64(msPerDay)
		forecastDays = max(int(math.Round(diff)), 1)
		endDayNumber = currentDayNumber + (forecastDays - 1)
	default:
		monthsStr := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(periodID, " Months"), "Month"), "Months")
		months, err := strconv.Atoi(strings.TrimSpace(monthsStr))
		if err != nil || months <= 0 {
			months = 1
		}
		endDate := currentDate.AddDate(0, months, 0)
		diff := float64(endDate.UnixMilli()-currentDate.UnixMilli()) / float64(msPerDay)
		forecastDays = max(int(math.Round(diff)), 1)
		endDayNumber = currentDayNumber + (forecastDays - 1)
	}

	actualForecastDays := endDayNumber - currentDayNumber + 1

	type dueRow struct {
		Due           int    `db:"due"            json:"due"`
		IntervalRange string `db:"interval_range" json:"interval_range"`
		Count         int    `db:"count"          json:"count"`
	}

	query := `
  SELECT
    due,
    CASE
      WHEN c.interval < 20 THEN 'learning'
      ELSE 'known'
    END as interval_range,
    COUNT(*) as count
  FROM card c
  JOIN card_type ct ON c.cardTypeId = ct.id
  WHERE ct.lang = ? AND c.due BETWEEN ? AND ? AND c.del = 0`

	params := []any{lang, currentDayNumber, endDayNumber}
	useDeckFilter := deckID != "" && deckID != cacheAllKey
	if useDeckFilter {
		query += deckIDClause
		params = append(params, deckID)
	}
	query += " GROUP BY due, interval_range ORDER BY due;"

	rows, err := runQuery[dueRow](ctx, client, query, params...)
	if err != nil {
		return nil, err
	}

	labels := make([]string, actualForecastDays)
	knownCounts := make([]int, actualForecastDays)
	learningCounts := make([]int, actualForecastDays)
	counts := make([]int, actualForecastDays)

	for i := range actualForecastDays {
		d := chartStartDate.AddDate(0, 0, currentDayNumber+i)
		labels[i] = d.Format("Jan 2, 2006")
	}

	for _, row := range rows {
		dayIndex := row.Due - currentDayNumber
		if dayIndex < 0 || dayIndex >= actualForecastDays {
			continue
		}
		switch row.IntervalRange {
		case "learning":
			learningCounts[dayIndex] += row.Count
		case "known":
			knownCounts[dayIndex] += row.Count
		}
		counts[dayIndex] += row.Count
	}

	if periodID == periodAllTime {
		lastNonZeroIndex := len(counts) - 1
		for lastNonZeroIndex >= 0 && counts[lastNonZeroIndex] == 0 {
			lastNonZeroIndex--
		}
		extraDays := 5
		if lastNonZeroIndex >= 0 {
			lastNonZeroIndex += extraDays
			if lastNonZeroIndex >= len(counts) {
				lastNonZeroIndex = len(counts) - 1
			}
			labels = labels[:lastNonZeroIndex+1]
			learningCounts = learningCounts[:lastNonZeroIndex+1]
			knownCounts = knownCounts[:lastNonZeroIndex+1]
			counts = counts[:lastNonZeroIndex+1]
		}
	}

	stats := &DueStats{
		Labels:         labels,
		Counts:         counts,
		KnownCounts:    knownCounts,
		LearningCounts: learningCounts,
	}

	s.cache.Set(cacheKey, stats)
	return stats, nil
}

func (s *MigakuService) GetIntervalStats(
	ctx context.Context,
	client *MigakuClient,
	lang, deckID, percentileID string,
) (*IntervalStats, error) {
	if lang == "" {
		return nil, errors.New("lang parameter is required")
	}

	if percentileID == "" {
		percentileID = "75th"
	}

	cacheKey := s.scopedCacheKey(client, fmt.Sprintf("stats:interval:%s:%s:%s", lang, deckID, percentileID))
	if cached, ok := s.cache.Get(cacheKey); ok {
		if is, ok := cached.(*IntervalStats); ok {
			return is, nil
		}
	}

	type intervalRow struct {
		IntervalGroup float64 `db:"interval_group" json:"interval_group"`
		Count         int     `db:"count"          json:"count"`
	}

	query := `
  SELECT
    ROUND(interval) as interval_group,
    COUNT(*) as count
  FROM card c
  JOIN card_type ct ON c.cardTypeId = ct.id
  WHERE ct.lang = ? AND c.del = 0 AND c.interval > 0`

	params := []any{lang}
	useDeckFilter := deckID != "" && deckID != cacheAllKey
	if useDeckFilter {
		query += deckIDClause
		params = append(params, deckID)
	}
	query += " GROUP BY interval_group ORDER BY interval_group;"

	rows, err := runQuery[intervalRow](ctx, client, query, params...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		stats := &IntervalStats{Labels: []string{}, Counts: []int{}}
		s.cache.Set(cacheKey, stats)
		return stats, nil
	}

	intervalMap := make(map[int]int)
	maxInterval := 0
	totalCards := 0
	for _, row := range rows {
		interval := int(row.IntervalGroup)
		count := row.Count
		intervalMap[interval] += count
		if interval > maxInterval {
			maxInterval = interval
		}
		totalCards += count
	}

	percentileNum, err := strconv.Atoi(strings.TrimSuffix(percentileID, "th"))
	if err != nil || percentileNum <= 0 {
		percentileNum = 75
	}
	cutoffPercentile := float64(percentileNum) / 100.0

	sortedIntervals := make([]int, 0, len(intervalMap))
	for k := range intervalMap {
		sortedIntervals = append(sortedIntervals, k)
	}
	sort.Ints(sortedIntervals)

	cumulativeCount := 0
	cutoffInterval := maxInterval
	for _, interval := range sortedIntervals {
		cumulativeCount += intervalMap[interval]
		var pc float64
		if totalCards > 0 {
			pc = float64(cumulativeCount) / float64(totalCards)
		} else {
			pc = 1
		}
		if pc >= cutoffPercentile {
			cutoffInterval = interval
			break
		}
	}

	labels := make([]string, 0, cutoffInterval)
	counts := make([]int, 0, cutoffInterval)
	for i := 1; i <= cutoffInterval; i++ {
		if i == 1 {
			labels = append(labels, "1 day")
		} else {
			labels = append(labels, fmt.Sprintf("%d days", i))
		}
		counts = append(counts, intervalMap[i])
	}

	stats := &IntervalStats{
		Labels: labels,
		Counts: counts,
	}
	s.cache.Set(cacheKey, stats)
	return stats, nil
}

func (s *MigakuService) GetStudyStats(
	ctx context.Context,
	client *MigakuClient,
	lang, deckID, periodID string,
) (*StudyStats, error) {
	if lang == "" {
		return nil, errors.New("lang parameter is required")
	}

	if periodID == "" {
		periodID = "1 Month"
	}

	cacheKey := s.scopedCacheKey(client, fmt.Sprintf("stats:study:%s:%s:%s", lang, deckID, periodID))
	if cached, ok := s.cache.Get(cacheKey); ok {
		if ss, ok := cached.(*StudyStats); ok {
			return ss, nil
		}
	}

	currentDate := time.Now()
	currentDate = time.Date(currentDate.Year(), currentDate.Month(), currentDate.Day(), 0, 0, 0, 0, currentDate.Location())
	startDate := time.Date(2020, time.January, 1, 0, 0, 0, 0, currentDate.Location())
	currentDelta := currentDate.UnixMilli() - startDate.UnixMilli()
	currentDayNumber := int(currentDelta / msPerDay)

	var periodDays int
	var startDayNumber int
	var earliestReviewDayForAllTime *int

	if periodID == periodAllTime {
		query := `
SELECT MIN(r.day) as minDay
FROM review r
JOIN card c ON r.cardId = c.id
JOIN card_type ct ON c.cardTypeId = ct.id
WHERE ct.lang = ? AND r.del = 0`
		params := []any{lang}
		useDeckFilter := deckID != "" && deckID != cacheAllKey
		if useDeckFilter {
			query += deckIDClause
			params = append(params, deckID)
		}

		type minDayRow struct {
			MinDay *int `db:"minDay" json:"minDay"`
		}

		rows, err := runQuery[minDayRow](ctx, client, query, params...)
		if err == nil && len(rows) > 0 && rows[0].MinDay != nil {
			earliestReviewDayForAllTime = rows[0].MinDay
			periodDays = currentDayNumber - *earliestReviewDayForAllTime + 1
			startDayNumber = *earliestReviewDayForAllTime
		} else {
			periodDays = currentDayNumber + 1
			startDayNumber = 0
		}
	} else {
		var months int
		if strings.Contains(periodID, "Year") {
			numStr := strings.TrimSpace(strings.TrimSuffix(strings.ReplaceAll(periodID, "Years", ""), "Year"))
			n, err := strconv.Atoi(numStr)
			if err != nil || n <= 0 {
				n = 1
			}
			months = n * 12
		} else {
			numStr := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(periodID, " Months"), "Month"), "Months"))
			n, err := strconv.Atoi(numStr)
			if err != nil || n <= 0 {
				n = 1
			}
			months = n
		}

		today := startDate.AddDate(0, 0, currentDayNumber)
		periodStartDate := today.AddDate(0, -months, 0)
		diff := float64(today.UnixMilli()-periodStartDate.UnixMilli()) / float64(msPerDay)
		periodDays = int(math.Round(diff)) + 1
		if periodDays <= 0 {
			periodDays = 1
		}
		startDayNumber = currentDayNumber - periodDays + 1
	}

	studyQuery := `
SELECT 
  COUNT(DISTINCT r.day) as days_studied,
  COUNT(*) as total_reviews
FROM review r
JOIN card c ON r.cardId = c.id
JOIN card_type ct ON c.cardTypeId = ct.id
WHERE ct.lang = ? AND r.day BETWEEN ? AND ? AND r.del = 0`
	studyParams := []any{lang, startDayNumber, currentDayNumber}

	// #nosec G101 -- SQL query string, no credentials.
	passRateQuery := `
SELECT 
  SUM(CASE WHEN r.type = 2 THEN 1 ELSE 0 END) as successful_reviews,
  SUM(CASE WHEN r.type = 1 THEN 1 ELSE 0 END) as failed_reviews
FROM review r
JOIN card c ON r.cardId = c.id
JOIN card_type ct ON c.cardTypeId = ct.id
WHERE ct.lang = ? AND r.day BETWEEN ? AND ? AND r.del = 0 AND r.type IN (1, 2)`
	passRateParams := []any{lang, startDayNumber, currentDayNumber}

	newCardsQuery := `
SELECT 
  COUNT(DISTINCT r.cardId) as new_cards_reviewed
FROM review r
JOIN card c ON r.cardId = c.id
JOIN card_type ct ON c.cardTypeId = ct.id
WHERE ct.lang = ? AND r.day BETWEEN ? AND ? AND r.del = 0 AND r.type = 0`
	newCardsParams := []any{lang, startDayNumber, currentDayNumber}

	cardsAddedQuery := `
SELECT 
  COUNT(*) as cards_added
FROM card c
JOIN card_type ct ON c.cardTypeId = ct.id
WHERE ct.lang = ? AND c.created >= ? AND c.created <= ? AND c.del = 0 AND c.lessonId = ''`

	startDayDate := startDate.AddDate(0, 0, startDayNumber)
	startDayDate = time.Date(startDayDate.Year(), startDayDate.Month(), startDayDate.Day(), 0, 0, 0, 0, startDayDate.Location())
	cardsAddedParams := []any{lang, startDayDate.UnixMilli(), time.Now().UnixMilli()}

	cardsLearnedQuery := `
SELECT 
  COUNT(DISTINCT c.id) as cards_learned
FROM review r
JOIN card c ON r.cardId = c.id
JOIN card_type ct ON c.cardTypeId = ct.id
WHERE ct.lang = ? AND r.day BETWEEN ? AND ? AND r.del = 0 
  AND c.interval >= 20 AND r.interval < 20 AND r.type = 2`
	cardsLearnedParams := []any{lang, startDayNumber, currentDayNumber}

	totalNewCardsQuery := `
SELECT 
  COUNT(DISTINCT r.cardId) as total_new_cards
FROM review r
JOIN card c ON r.cardId = c.id
JOIN card_type ct ON c.cardTypeId = ct.id
WHERE ct.lang = ? AND r.day BETWEEN ? AND ? AND c.del = 0 AND r.del = 0 AND r.type = 0`
	totalNewCardsParams := []any{lang, startDayNumber, currentDayNumber}

	cardsLearnedPerDayQuery := `
SELECT 
  ROUND(COUNT(DISTINCT c.id) * 1.0 / NULLIF(COUNT(DISTINCT r.day), 0), 1) as cards_learned_per_day
FROM review r
JOIN card c ON r.cardId = c.id
JOIN card_type ct ON c.cardTypeId = ct.id
WHERE ct.lang = ? AND r.day BETWEEN ? AND ? AND r.del = 0 
  AND c.interval >= 20 AND r.interval < 20 AND r.type = 2`
	cardsLearnedPerDayParams := []any{lang, startDayNumber, currentDayNumber}

	newCardsTimeQuery := `
SELECT 
  SUM(r.duration) as total_time_seconds,
  COUNT(*) as review_count,
  ROUND(AVG(r.duration), 1) as avg_time_seconds
FROM review r
JOIN card c ON r.cardId = c.id
JOIN card_type ct ON c.cardTypeId = ct.id
WHERE ct.lang = ? AND r.day BETWEEN ? AND ? AND r.del = 0 AND r.type = 0`
	newCardsTimeParams := []any{lang, startDayNumber, currentDayNumber}

	reviewsTimeQuery := `
SELECT 
  SUM(r.duration) as total_time_seconds,
  COUNT(*) as review_count,
  ROUND(AVG(r.duration), 1) as avg_time_seconds
FROM review r
JOIN card c ON r.cardId = c.id
JOIN card_type ct ON c.cardTypeId = ct.id
WHERE ct.lang = ? AND r.day BETWEEN ? AND ? AND r.del = 0 AND r.type IN (1, 2)`
	reviewsTimeParams := []any{lang, startDayNumber, currentDayNumber}

	useDeckFilter := deckID != "" && deckID != cacheAllKey
	if useDeckFilter {
		studyQuery += deckIDClause
		studyParams = append(studyParams, deckID)

		// #nosec G101 -- SQL query string, no credentials.
		passRateQuery += deckIDClause
		passRateParams = append(passRateParams, deckID)

		newCardsQuery += deckIDClause
		newCardsParams = append(newCardsParams, deckID)

		cardsAddedQuery += deckIDClause
		cardsAddedParams = append(cardsAddedParams, deckID)

		cardsLearnedQuery += deckIDClause
		cardsLearnedParams = append(cardsLearnedParams, deckID)

		totalNewCardsQuery += deckIDClause
		totalNewCardsParams = append(totalNewCardsParams, deckID)

		cardsLearnedPerDayQuery += deckIDClause
		cardsLearnedPerDayParams = append(cardsLearnedPerDayParams, deckID)

		newCardsTimeQuery += deckIDClause
		newCardsTimeParams = append(newCardsTimeParams, deckID)

		reviewsTimeQuery += deckIDClause
		reviewsTimeParams = append(reviewsTimeParams, deckID)
	}

	type studyRow struct {
		DaysStudied  int `db:"days_studied"  json:"days_studied"`
		TotalReviews int `db:"total_reviews" json:"total_reviews"`
	}

	type passRateRow struct {
		SuccessfulReviews int `db:"successful_reviews" json:"successful_reviews"`
		FailedReviews     int `db:"failed_reviews"     json:"failed_reviews"`
	}

	type newCardsRow struct {
		NewCardsReviewed int `db:"new_cards_reviewed" json:"new_cards_reviewed"`
	}

	type cardsAddedRow struct {
		CardsAdded int `db:"cards_added" json:"cards_added"`
	}

	type cardsLearnedRow struct {
		CardsLearned int `db:"cards_learned" json:"cards_learned"`
	}

	type totalNewCardsRow struct {
		TotalNewCards int `db:"total_new_cards" json:"total_new_cards"`
	}

	type cardsLearnedPerDayRow struct {
		CardsLearnedPerDay float64 `db:"cards_learned_per_day" json:"cards_learned_per_day"`
	}

	type timeRow struct {
		TotalTimeSeconds int     `db:"total_time_seconds" json:"total_time_seconds"`
		ReviewCount      int     `db:"review_count"       json:"review_count"`
		AvgTimeSeconds   float64 `db:"avg_time_seconds"   json:"avg_time_seconds"`
	}

	studyResults, err := runQuery[studyRow](ctx, client, studyQuery, studyParams...)
	if err != nil {
		return nil, err
	}
	passRateResults, err := runQuery[passRateRow](ctx, client, passRateQuery, passRateParams...)
	if err != nil {
		return nil, err
	}
	newCardsResults, err := runQuery[newCardsRow](ctx, client, newCardsQuery, newCardsParams...)
	if err != nil {
		return nil, err
	}
	cardsAddedResults, err := runQuery[cardsAddedRow](ctx, client, cardsAddedQuery, cardsAddedParams...)
	if err != nil {
		return nil, err
	}
	cardsLearnedResults, err := runQuery[cardsLearnedRow](ctx, client, cardsLearnedQuery, cardsLearnedParams...)
	if err != nil {
		return nil, err
	}
	totalNewCardsResults, err := runQuery[totalNewCardsRow](ctx, client, totalNewCardsQuery, totalNewCardsParams...)
	if err != nil {
		return nil, err
	}
	cardsLearnedPerDayResults, err := runQuery[cardsLearnedPerDayRow](
		ctx,
		client,
		cardsLearnedPerDayQuery,
		cardsLearnedPerDayParams...,
	)
	if err != nil {
		return nil, err
	}
	newCardsTimeResults, err := runQuery[timeRow](ctx, client, newCardsTimeQuery, newCardsTimeParams...)
	if err != nil {
		return nil, err
	}
	reviewsTimeResults, err := runQuery[timeRow](ctx, client, reviewsTimeQuery, reviewsTimeParams...)
	if err != nil {
		return nil, err
	}

	daysStudied := 0
	totalReviews := 0
	if len(studyResults) > 0 {
		row := studyResults[0]
		daysStudied = row.DaysStudied
		totalReviews = row.TotalReviews
	}

	var denominator int
	if periodID == periodAllTime && daysStudied > 0 && earliestReviewDayForAllTime != nil {
		denominator = currentDayNumber - *earliestReviewDayForAllTime + 1
	} else {
		if periodDays <= 0 {
			periodDays = 1
		}
		denominator = periodDays
	}
	daysStudiedPercent := 0
	if denominator > 0 {
		daysStudiedPercent = int(math.Round((float64(daysStudied) / float64(denominator)) * 100))
	}

	passRate := 0
	if len(passRateResults) > 0 {
		row := passRateResults[0]
		successful := row.SuccessfulReviews
		failed := row.FailedReviews
		totalAnswered := successful + failed
		if totalAnswered > 0 && successful > 0 {
			passRate = int(math.Round((float64(successful-failed) / float64(successful)) * 100))
		}
	}

	newCardsReviewed := 0
	if len(newCardsResults) > 0 {
		newCardsReviewed = newCardsResults[0].NewCardsReviewed
	}

	if periodDays <= 0 {
		periodDays = 1
	}

	newCardsPerDay := float64(newCardsReviewed) / float64(periodDays)
	newCardsPerDay = math.Round(newCardsPerDay*10) / 10

	totalCardsAdded := 0
	if len(cardsAddedResults) > 0 {
		totalCardsAdded = cardsAddedResults[0].CardsAdded
	}
	cardsAddedPerDay := 0.0
	if totalCardsAdded > 0 {
		cardsAddedPerDay = float64(totalCardsAdded) / float64(periodDays)
		cardsAddedPerDay = math.Round(cardsAddedPerDay*10) / 10
	}

	totalCardsLearned := 0
	if len(cardsLearnedResults) > 0 {
		totalCardsLearned = cardsLearnedResults[0].CardsLearned
	}

	totalNewCards := 0
	if len(totalNewCardsResults) > 0 {
		totalNewCards = totalNewCardsResults[0].TotalNewCards
	}

	cardsLearnedPerDay := 0.0
	if len(cardsLearnedPerDayResults) > 0 {
		cardsLearnedPerDay = cardsLearnedPerDayResults[0].CardsLearnedPerDay
	}

	avgReviewsPerCalendarDay := 0.0
	if totalReviews > 0 {
		avgReviewsPerCalendarDay = float64(totalReviews) / float64(periodDays)
		avgReviewsPerCalendarDay = math.Round(avgReviewsPerCalendarDay*10) / 10
	}

	totalTimeNewCardsSeconds := 0
	avgTimeNewCardSeconds := 0.0
	if len(newCardsTimeResults) > 0 {
		row := newCardsTimeResults[0]
		totalTimeNewCardsSeconds = row.TotalTimeSeconds
		avgTimeNewCardSeconds = row.AvgTimeSeconds
	}

	totalTimeReviewsSeconds := 0
	avgTimeReviewSeconds := 0.0
	if len(reviewsTimeResults) > 0 {
		row := reviewsTimeResults[0]
		totalTimeReviewsSeconds = row.TotalTimeSeconds
		avgTimeReviewSeconds = row.AvgTimeSeconds
	}

	stats := &StudyStats{
		DaysStudied:              daysStudied,
		DaysStudiedPercent:       daysStudiedPercent,
		TotalReviews:             totalReviews,
		AvgReviewsPerCalendarDay: avgReviewsPerCalendarDay,
		PeriodDays:               periodDays,
		PassRate:                 passRate,
		NewCardsPerDay:           newCardsPerDay,
		TotalNewCards:            totalNewCards,
		TotalCardsAdded:          totalCardsAdded,
		CardsAddedPerDay:         cardsAddedPerDay,
		TotalCardsLearned:        totalCardsLearned,
		CardsLearnedPerDay:       cardsLearnedPerDay,
		TotalTimeNewCardsSeconds: totalTimeNewCardsSeconds,
		AvgTimeNewCardSeconds:    avgTimeNewCardSeconds,
		TotalTimeReviewsSeconds:  totalTimeReviewsSeconds,
		AvgTimeReviewSeconds:     avgTimeReviewSeconds,
	}

	s.cache.Set(cacheKey, stats)
	return stats, nil
}

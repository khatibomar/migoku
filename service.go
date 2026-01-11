package main

import (
	"errors"
	"fmt"
)

// Word represents a word in the domain
type Word struct {
	DictForm    string `json:"dictForm"`
	Secondary   string `json:"secondary"`
	KnownStatus string `json:"knownStatus,omitempty"`
}

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

// NewMigakuService creates a new service instance
func NewMigakuService(repo *Repository, cache *Cache) *MigakuService {
	return &MigakuService{
		repo:  repo,
		cache: cache,
	}
}

// GetWords retrieves words with optional status and language filters
func (s *MigakuService) GetWords(lang, status string) ([]Word, error) {
	if status != "" && status != "known" && status != "learning" && status != "unknown" && status != "ignored" {
		return nil, errors.New("invalid status: must be one of: known, learning, unknown, ignored")
	}

	cacheKey := "words:"
	if status == "" {
		cacheKey += "all:"
	} else {
		cacheKey += status + ":"
	}
	if lang == "" {
		cacheKey += "all"
	} else {
		cacheKey += lang
	}

	if cached, ok := s.cache.Get(cacheKey); ok {
		if words, ok := cached.([]Word); ok {
			return words, nil
		}
	}

	var dbStatus string
	if status != "" {
		switch status {
		case "known":
			dbStatus = "KNOWN"
		case "learning":
			dbStatus = "LEARNING"
		case "unknown":
			dbStatus = "UNKNOWN"
		case "ignored":
			dbStatus = "IGNORED"
		}
	}

	limit := 0
	if dbStatus == "" {
		limit = 10000
	}

	rows, err := s.repo.GetWords(lang, dbStatus, limit)
	if err != nil {
		return nil, err
	}

	words := WordsFromRows(rows)
	s.cache.Set(cacheKey, words)

	return words, nil
}

// GetDecks retrieves all decks with caching
func (s *MigakuService) GetDecks() ([]Deck, error) {
	cacheKey := "decks"

	if cached, ok := s.cache.Get(cacheKey); ok {
		if decks, ok := cached.([]Deck); ok {
			return decks, nil
		}
	}

	rows, err := s.repo.GetDecks()
	if err != nil {
		return nil, err
	}

	decks := DecksFromRows(rows)
	s.cache.Set(cacheKey, decks)

	return decks, nil
}

// GetStatusCounts retrieves status counts with caching
func (s *MigakuService) GetStatusCounts(lang, deckID string) (*StatusCounts, error) {
	cacheKey := s.buildStatusCountsCacheKey(lang, deckID)

	if cached, ok := s.cache.Get(cacheKey); ok {
		if counts, ok := cached.(*StatusCounts); ok {
			return counts, nil
		}
	}

	rows, err := s.repo.GetStatusCounts(lang, deckID)
	if err != nil {
		return nil, err
	}

	counts := StatusCountsFromRows(rows)
	s.cache.Set(cacheKey, &counts)

	return &counts, nil
}

// GetTables retrieves all database tables with caching
func (s *MigakuService) GetTables() ([]Table, error) {
	cacheKey := "tables"

	if cached, ok := s.cache.Get(cacheKey); ok {
		if tables, ok := cached.([]Table); ok {
			return tables, nil
		}
	}

	rows, err := s.repo.GetTables()
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
		cacheKey += "all:"
	} else {
		cacheKey += fmt.Sprintf("deck:%s:", deckID)
	}

	if lang == "" {
		cacheKey += "all"
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
func (s *MigakuService) GetDifficultWords(lang string, limit int, deckID string) ([]DifficultWord, error) {
	if limit == 0 {
		limit = 50
	}
	cacheKey := fmt.Sprintf("difficult:words:%s:%d:%s", lang, limit, deckID)

	if cached, ok := s.cache.Get(cacheKey); ok {
		if words, ok := cached.([]DifficultWord); ok {
			return words, nil
		}
	}

	rows, err := s.repo.GetDifficultWords(lang, limit, deckID)
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
func (s *MigakuService) GetDatabaseSchema() (DatabaseSchema, error) {
	cacheKey := "database:schema"

	if cached, ok := s.cache.Get(cacheKey); ok {
		if schema, ok := cached.(DatabaseSchema); ok {
			return schema, nil
		}
	}

	rows, err := s.repo.GetDatabaseSchema()
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

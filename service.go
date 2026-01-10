package main

import "fmt"

// Word represents a word in the domain
type Word struct {
	DictForm    string `json:"dictForm"`
	Secondary   string `json:"secondary"`
	KnownStatus string `json:"knownStatus,omitempty"`
}

// WordFromRow creates a Word from a repository wordRow
func WordFromRow(row wordRow) Word {
	return Word{
		DictForm:    row.DictForm,
		Secondary:   row.Secondary,
		KnownStatus: row.KnownStatus,
	}
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
	return Deck{
		ID:   row.ID,
		Name: row.Name,
	}
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
	return Table{
		Name: row.Name,
	}
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

// GetAllWords retrieves all words with caching
func (s *MigakuService) GetAllWords(lang string) ([]Word, error) {
	cacheKey := "words:all:"
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

	rows, err := s.repo.GetWords(lang, "", 10000)
	if err != nil {
		return nil, err
	}

	words := WordsFromRows(rows)
	s.cache.Set(cacheKey, words)

	return words, nil
}

// GetKnownWords retrieves known words with caching
func (s *MigakuService) GetKnownWords(lang string) ([]Word, error) {
	cacheKey := "words:known:"
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

	rows, err := s.repo.GetWords(lang, "KNOWN", 0)
	if err != nil {
		return nil, err
	}

	words := WordsFromRows(rows)
	s.cache.Set(cacheKey, words)

	return words, nil
}

// GetLearningWords retrieves learning words with caching
func (s *MigakuService) GetLearningWords(lang string) ([]Word, error) {
	cacheKey := "words:learning:"
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

	rows, err := s.repo.GetWords(lang, "LEARNING", 0)
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

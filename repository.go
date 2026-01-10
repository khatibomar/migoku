package main

import "fmt"

// wordRow represents a word row from the WordList table
type wordRow struct {
	DictForm    string `json:"dictForm"`
	Secondary   string `json:"secondary"`
	KnownStatus string `json:"knownStatus,omitempty"`
}

// deckRow represents a deck row from the deck table
type deckRow struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// tableRow represents a table name from sqlite_master
type tableRow struct {
	Name string `json:"name"`
}

// statusCountRow represents a single row from the GROUP BY query
type statusCountRow struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// Repository handles database operations
type Repository struct {
	app *Application
}

// NewRepository creates a new repository instance
func NewRepository(app *Application) *Repository {
	return &Repository{app: app}
}

// GetWords retrieves words from WordList with optional filters
// status can be empty for all words, or "KNOWN", "LEARNING", etc.
// limit can be 0 for no limit
func (r *Repository) GetWords(lang, status string, limit int) ([]wordRow, error) {
	var query string
	var params []any

	query = "SELECT dictForm, secondary, knownStatus FROM WordList WHERE del = 0"

	if lang != "" {
		query += " AND language = ?"
		params = append(params, lang)
	}

	if status != "" {
		query += " AND knownStatus = ?"
		params = append(params, status)
	}

	if limit > 0 {
		query += " LIMIT ?"
		params = append(params, limit)
	}

	query += ";"

	words, err := runQuery[wordRow](r.app, query, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to get words: %w", err)
	}

	return words, nil
}

// GetDecks retrieves all active decks
func (r *Repository) GetDecks() ([]deckRow, error) {
	query := "SELECT id, name FROM deck WHERE del = 0 ORDER BY name;"
	decks, err := runQuery[deckRow](r.app, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get decks: %w", err)
	}

	return decks, nil
}

// GetStatusCounts retrieves status counts with optional filters
func (r *Repository) GetStatusCounts(lang, deckID string) ([]statusCountRow, error) {
	var params []any

	query := "SELECT knownStatus as status, count(1) as count FROM WordList WHERE del = 0"

	if deckID != "" {
		query += " AND deckId = ?"
		params = append(params, deckID)
	}

	if lang != "" {
		query += " AND language = ?"
		params = append(params, lang)
	}

	query += " GROUP BY knownStatus;"

	rows, err := runQuery[statusCountRow](r.app, query, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to get status counts: %w", err)
	}

	return rows, nil
}

// GetTables retrieves all database tables
func (r *Repository) GetTables() ([]tableRow, error) {
	query := "SELECT name FROM sqlite_master WHERE type='table';"
	tables, err := runQuery[tableRow](r.app, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get tables: %w", err)
	}

	return tables, nil
}

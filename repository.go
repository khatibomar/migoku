package main

import (
	"context"
	"fmt"
)

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

const deckIDClause = " AND c.deckId = ?"

// Repository handles database operations
type Repository struct{}

// NewRepository creates a new repository instance
func NewRepository() *Repository {
	return &Repository{}
}

// GetWords retrieves words from WordList with optional filters
// status can be empty for all words, or "KNOWN", "LEARNING", etc.
// limit can be 0 for no limit
func (r *Repository) GetWords(ctx context.Context, browser *Browser, lang, status string, limit int) ([]wordRow, error) {
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

	words, err := runQuery[wordRow](ctx, browser, query, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to get words: %w", err)
	}

	return words, nil
}

// GetDecks retrieves all active decks
func (r *Repository) GetDecks(ctx context.Context, browser *Browser) ([]deckRow, error) {
	query := "SELECT id, name FROM deck WHERE del = 0 ORDER BY name;"
	decks, err := runQuery[deckRow](ctx, browser, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get decks: %w", err)
	}

	return decks, nil
}

// GetStatusCounts retrieves status counts with optional filters
func (r *Repository) GetStatusCounts(ctx context.Context, browser *Browser, lang, deckID string) ([]statusCountRow, error) {
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

	rows, err := runQuery[statusCountRow](ctx, browser, query, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to get status counts: %w", err)
	}

	return rows, nil
}

// GetTables retrieves all database tables
func (r *Repository) GetTables(ctx context.Context, browser *Browser) ([]tableRow, error) {
	query := "SELECT name FROM sqlite_master WHERE type='table';"
	tables, err := runQuery[tableRow](ctx, browser, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get tables: %w", err)
	}

	return tables, nil
}

// difficultWordRow represents words with high fail rates
type difficultWordRow struct {
	DictForm      string  `json:"dictForm"`
	Secondary     string  `json:"secondary"`
	PartOfSpeech  string  `json:"partOfSpeech"`
	KnownStatus   string  `json:"knownStatus"`
	TotalReviews  int     `json:"total_reviews"`
	FailedReviews int     `json:"failed_reviews"`
	FailRate      float64 `json:"fail_rate"`
}

// GetDifficultWords retrieves words with highest fail rates (min 5 reviews)
func (r *Repository) GetDifficultWords(
	ctx context.Context,
	browser *Browser,
	lang string,
	limit int,
	deckID string,
) ([]difficultWordRow, error) {
	var params []any
	query := `SELECT 
	            w.dictForm,
	            w.secondary,
	            w.partOfSpeech,
	            w.knownStatus,
	            COUNT(r.id) as total_reviews,
	            SUM(CASE WHEN r.type = 1 THEN 1 ELSE 0 END) as failed_reviews,
	            ROUND(CAST(SUM(CASE WHEN r.type = 1 THEN 1 ELSE 0 END) AS FLOAT) / COUNT(r.id) * 100, 2) as fail_rate
	          FROM WordList w
	          JOIN CardWordRelation cwr ON w.dictForm = cwr.dictForm
			  	AND w.secondary = cwr.secondary AND w.partOfSpeech = cwr.partOfSpeech
	          JOIN card c ON cwr.cardId = c.id
	          JOIN review r ON c.id = r.cardId
	          WHERE w.language = ? AND w.del = 0 AND c.del = 0 AND r.del = 0 AND r.type IN (1, 2)`

	params = append(params, lang)

	if deckID != "" {
		query += deckIDClause
		params = append(params, deckID)
	}

	query += `
	          GROUP BY w.dictForm, w.secondary, w.partOfSpeech
	          HAVING total_reviews >= 5
	          ORDER BY fail_rate DESC, total_reviews DESC
	          LIMIT ?;`

	params = append(params, limit)

	words, err := runQuery[difficultWordRow](ctx, browser, query, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to get difficult words: %w", err)
	}
	return words, nil
}

// schemaRow represents database schema information
type schemaRow struct {
	TableName    string `json:"table_name"`
	ColumnName   string `json:"column_name"`
	ColumnType   string `json:"column_type"`
	IsNotNull    int    `json:"is_not_null"`
	IsPrimaryKey int    `json:"is_pk"`
}

// GetDatabaseSchema retrieves the database schema from sqlite_master
func (r *Repository) GetDatabaseSchema(ctx context.Context, browser *Browser) ([]schemaRow, error) {
	query := `SELECT
	            m.name AS table_name,
	            p.name AS column_name,
	            p.type AS column_type,
	            p."notnull" AS is_not_null,
	            p.pk AS is_pk
	          FROM sqlite_master m
	          JOIN pragma_table_info(m.name) p
	          WHERE m.type = 'table'
	          ORDER BY m.name, p.cid;`

	schema, err := runQuery[schemaRow](ctx, browser, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get database schema: %w", err)
	}
	return schema, nil
}

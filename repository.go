package main

import (
	"context"
	"fmt"
)

// wordRow represents a word row from the WordList table
type wordRow struct {
	DictForm    string `db:"dictForm"    json:"dictForm"`
	Secondary   string `db:"secondary"   json:"secondary"`
	KnownStatus string `db:"knownStatus" json:"knownStatus,omitempty"`
}

// deckRow represents a deck row from the deck table
type deckRow struct {
	ID   int    `db:"id"   json:"id"`
	Name string `db:"name" json:"name"`
}

// tableRow represents a table name from sqlite_master
type tableRow struct {
	Name string `db:"name" json:"name"`
}

// statusCountRow represents a single row from the GROUP BY query
type statusCountRow struct {
	Status string `db:"status" json:"status"`
	Count  int    `db:"count"  json:"count"`
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
func (r *Repository) GetWords(
	ctx context.Context,
	client *MigakuClient,
	lang, status string,
	limit int,
	deckID string,
) ([]wordRow, error) {
	var query string
	var params []any

	if deckID != "" {
		query = `SELECT DISTINCT w.dictForm, w.secondary, w.knownStatus
			FROM WordList w
			JOIN CardWordRelation cwr
				ON w.dictForm = cwr.dictForm
				AND w.secondary = cwr.secondary
				AND w.partOfSpeech = cwr.partOfSpeech
				AND w.language = cwr.language
			JOIN card c ON cwr.cardId = c.id
			WHERE w.del = 0 AND c.del = 0 AND c.deckId = ?`
		params = append(params, deckID)
	} else {
		query = "SELECT dictForm, secondary, knownStatus FROM WordList WHERE del = 0"
	}

	if lang != "" {
		if deckID != "" {
			query += " AND w.language = ?"
		} else {
			query += " AND language = ?"
		}
		params = append(params, lang)
	}

	if status != "" {
		if deckID != "" {
			query += " AND w.knownStatus = ?"
		} else {
			query += " AND knownStatus = ?"
		}
		params = append(params, status)
	}

	if limit > 0 {
		query += " LIMIT ?"
		params = append(params, limit)
	}

	query += ";"

	words, err := runQuery[wordRow](ctx, client, query, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to get words: %w", err)
	}

	return words, nil
}

// GetDecks retrieves all active decks
func (r *Repository) GetDecks(ctx context.Context, client *MigakuClient) ([]deckRow, error) {
	query := "SELECT id, name FROM deck WHERE del = 0 ORDER BY name;"
	decks, err := runQuery[deckRow](ctx, client, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get decks: %w", err)
	}

	return decks, nil
}

// GetStatusCounts retrieves status counts with optional filters
func (r *Repository) GetStatusCounts(ctx context.Context, client *MigakuClient, lang, deckID string) ([]statusCountRow, error) {
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

	rows, err := runQuery[statusCountRow](ctx, client, query, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to get status counts: %w", err)
	}

	return rows, nil
}

// GetTables retrieves all database tables
func (r *Repository) GetTables(ctx context.Context, client *MigakuClient) ([]tableRow, error) {
	query := "SELECT name FROM sqlite_master WHERE type='table';"
	tables, err := runQuery[tableRow](ctx, client, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get tables: %w", err)
	}

	return tables, nil
}

// difficultWordRow represents words with high fail rates
type difficultWordRow struct {
	DictForm      string  `db:"dictForm"       json:"dictForm"`
	Secondary     string  `db:"secondary"      json:"secondary"`
	PartOfSpeech  string  `db:"partOfSpeech"   json:"partOfSpeech"`
	KnownStatus   string  `db:"knownStatus"    json:"knownStatus"`
	TotalReviews  int     `db:"total_reviews"  json:"total_reviews"`
	FailedReviews int     `db:"failed_reviews" json:"failed_reviews"`
	FailRate      float64 `db:"fail_rate"      json:"fail_rate"`
}

// GetDifficultWords retrieves words with highest fail rates (min 5 reviews)
func (r *Repository) GetDifficultWords(
	ctx context.Context,
	client *MigakuClient,
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

	words, err := runQuery[difficultWordRow](ctx, client, query, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to get difficult words: %w", err)
	}
	return words, nil
}

// schemaRow represents database schema information
type schemaRow struct {
	TableName    string `db:"table_name"  json:"table_name"`
	ColumnName   string `db:"column_name" json:"column_name"`
	ColumnType   string `db:"column_type" json:"column_type"`
	IsNotNull    int    `db:"is_not_null" json:"is_not_null"`
	IsPrimaryKey int    `db:"is_pk"       json:"is_pk"`
}

// GetDatabaseSchema retrieves the database schema from sqlite_master
func (r *Repository) GetDatabaseSchema(ctx context.Context, client *MigakuClient) ([]schemaRow, error) {
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

	schema, err := runQuery[schemaRow](ctx, client, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get database schema: %w", err)
	}
	return schema, nil
}

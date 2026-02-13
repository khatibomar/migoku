package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

var (
	ErrWordNotFound     = errors.New("word not found")
	ErrInvalidStatus    = errors.New("invalid status: must be one of: known, learning, tracked, ignored")
	ErrWordTextRequired = errors.New("wordText is required")
	ErrClientNotAuth    = errors.New("client not authenticated")
)

type WordStatusItem struct {
	WordText  string `json:"wordText"`
	Secondary string `json:"secondary,omitempty"`
}

type wordRecord struct {
	DictForm         sql.NullString `db:"dictForm"`
	Secondary        sql.NullString `db:"secondary"`
	PartOfSpeech     sql.NullString `db:"partOfSpeech"`
	Language         sql.NullString `db:"language"`
	ServerMod        sql.NullInt64  `db:"serverMod"`
	KnownStatus      sql.NullString `db:"knownStatus"`
	HasCard          sql.NullBool   `db:"hasCard"`
	Tracked          sql.NullBool   `db:"tracked"`
	Created          sql.NullInt64  `db:"created"`
	Del              sql.NullInt64  `db:"del"`
	IsModern         sql.NullInt64  `db:"isModern"`
	ServerVersion    sql.NullInt64  `db:"serverVersion"`
	IsPendingEnqueue sql.NullInt64  `db:"isPendingEnqueue"`
	IsPendingApply   sql.NullInt64  `db:"isPendingApply"`
}

type wordStatusUpdate struct {
	KnownStatus string
	Tracked     bool
}

const languageFilterClause = " AND language = ?"

func statusToUpdate(status string) (wordStatusUpdate, bool) {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch normalized {
	case "known":
		return wordStatusUpdate{KnownStatus: "KNOWN", Tracked: false}, true
	case "learning":
		return wordStatusUpdate{KnownStatus: "LEARNING", Tracked: false}, true
	case "ignored":
		return wordStatusUpdate{KnownStatus: "IGNORED", Tracked: false}, true
	case "tracked":
		return wordStatusUpdate{KnownStatus: "UNKNOWN", Tracked: true}, true
	default:
		return wordStatusUpdate{}, false
	}
}

func (s *MigakuService) SetWordStatus(
	ctx context.Context,
	client *MigakuClient,
	wordText, secondary, status, language string,
) error {
	wordText = strings.TrimSpace(wordText)
	secondary = strings.TrimSpace(secondary)
	client.logger.Info(
		"Updating word status",
		slog.String("status", status),
		slog.String("wordText", wordText),
		slog.String("secondary", secondary),
		slog.String("language", language),
	)

	return s.setWordStatusItems(ctx, client, []WordStatusItem{
		{
			WordText:  wordText,
			Secondary: secondary,
		},
	}, status, language)
}

func (s *MigakuService) SetWordStatusBatch(
	ctx context.Context,
	client *MigakuClient,
	items []WordStatusItem,
	status string,
	language string,
) error {
	client.logger.Info(
		"Updating word status batch",
		slog.String("status", status),
		slog.Int("count", len(items)),
	)
	return s.setWordStatusItems(ctx, client, items, status, language)
}

func (s *MigakuService) setWordStatusItems(
	ctx context.Context,
	client *MigakuClient,
	items []WordStatusItem,
	status string,
	language string,
) error {
	if client == nil {
		return ErrClientNotAuth
	}

	update, ok := statusToUpdate(status)
	if !ok {
		return ErrInvalidStatus
	}

	if len(items) == 0 {
		return ErrWordTextRequired
	}

	normalizedItems := make([]WordStatusItem, 0, len(items))
	for _, item := range items {
		wordText := strings.TrimSpace(item.WordText)
		secondary := strings.TrimSpace(item.Secondary)
		if wordText == "" {
			return ErrWordTextRequired
		}
		normalizedItems = append(normalizedItems, WordStatusItem{
			WordText:  wordText,
			Secondary: secondary,
		})
	}

	updates := make([]map[string]any, 0, len(normalizedItems))
	updateRecords := make([]wordRecord, 0, len(normalizedItems))
	modTimestamp := time.Now().UnixMilli()

	if err := client.refreshDBIfStale(ctx, s.cache.ttl); err != nil {
		return err
	}

	for _, item := range normalizedItems {
		record, payload, recErr := lookupWordRecord(ctx, client, item.WordText, item.Secondary, language)
		if recErr != nil {
			return fmt.Errorf("%w: %s", ErrWordNotFound, item.WordText)
		}

		serverMod := int64(-1)
		if record.ServerMod.Valid {
			serverMod = record.ServerMod.Int64
		}

		if record.HasCard.Valid {
			payload["hasCard"] = record.HasCard.Bool
		} else {
			delete(payload, "hasCard")
		}

		payload["knownStatus"] = update.KnownStatus
		payload["tracked"] = update.Tracked
		payload["mod"] = modTimestamp
		payload["serverMod"] = serverMod
		updates = append(updates, payload)
		updateRecords = append(updateRecords, record)
	}

	if err := client.session.PushSync(ctx, updates); err != nil {
		return fmt.Errorf("failed to sync: %w", err)
	}

	if err := updateLocalWordStatus(ctx, client, updateRecords, update, modTimestamp); err != nil {
		return fmt.Errorf("failed to update local db: %w", err)
	}

	s.cache.Clear()
	return nil
}

func lookupWordRecord(
	ctx context.Context,
	client *MigakuClient,
	wordText, secondary, language string,
) (wordRecord, map[string]any, error) {
	query := `SELECT dictForm, secondary, partOfSpeech, language, serverMod, knownStatus, hasCard, tracked,
created, del, isModern, serverVersion, isPendingEnqueue, isPendingApply
FROM WordList
WHERE del = 0 AND dictForm = ?`
	params := []any{wordText}
	if strings.TrimSpace(language) != "" {
		query += languageFilterClause
		params = append(params, language)
	}
	if strings.TrimSpace(secondary) != "" {
		query += " AND secondary = ?"
		params = append(params, secondary)
	} else {
		query += " AND (secondary = '' OR secondary IS NULL)"
	}
	query += " LIMIT 1;"

	raw, err := runReadRow(ctx, client, query, params...)
	if err != nil {
		return wordRecord{}, nil, fmt.Errorf("word not found: %w", err)
	}

	payload := normalizeRow(raw)
	record := wordRecord{
		DictForm:         getNullString(payload, "dictForm"),
		Secondary:        getNullString(payload, "secondary"),
		PartOfSpeech:     getNullString(payload, "partOfSpeech"),
		Language:         getNullString(payload, "language"),
		ServerMod:        getNullInt64(payload, "serverMod"),
		KnownStatus:      getNullString(payload, "knownStatus"),
		HasCard:          getNullBool(payload, "hasCard"),
		Tracked:          getNullBool(payload, "tracked"),
		Created:          getNullInt64(payload, "created"),
		Del:              getNullInt64(payload, "del"),
		IsModern:         getNullInt64(payload, "isModern"),
		ServerVersion:    getNullInt64(payload, "serverVersion"),
		IsPendingEnqueue: getNullInt64(payload, "isPendingEnqueue"),
		IsPendingApply:   getNullInt64(payload, "isPendingApply"),
	}

	return record, payload, nil
}

func normalizeRow(raw map[string]any) map[string]any {
	result := make(map[string]any, len(raw))
	for key, value := range raw {
		if value == nil {
			continue
		}
		switch v := value.(type) {
		case []byte:
			result[key] = string(v)
		case int64:
			result[key] = coerceInt64Value(key, v)
		case int:
			result[key] = coerceInt64Value(key, int64(v))
		case float64:
			result[key] = coerceInt64Value(key, int64(v))
		default:
			result[key] = v
		}
	}
	return result
}

func coerceInt64Value(key string, value int64) any {
	switch key {
	case "hasCard", "tracked", "isModern", "isPendingEnqueue", "isPendingApply":
		return value != 0
	default:
		return value
	}
}

func getNullString(row map[string]any, key string) sql.NullString {
	value, ok := row[key]
	if !ok || value == nil {
		return sql.NullString{}
	}
	if v, ok := value.(string); ok {
		return sql.NullString{String: v, Valid: true}
	}
	return sql.NullString{String: fmt.Sprint(value), Valid: true}
}

func getNullInt64(row map[string]any, key string) sql.NullInt64 {
	value, ok := row[key]
	if !ok || value == nil {
		return sql.NullInt64{}
	}
	switch v := value.(type) {
	case int64:
		return sql.NullInt64{Int64: v, Valid: true}
	case int:
		return sql.NullInt64{Int64: int64(v), Valid: true}
	case float64:
		return sql.NullInt64{Int64: int64(v), Valid: true}
	default:
		return sql.NullInt64{}
	}
}

func getNullBool(row map[string]any, key string) sql.NullBool {
	value, ok := row[key]
	if !ok || value == nil {
		return sql.NullBool{}
	}
	switch v := value.(type) {
	case bool:
		return sql.NullBool{Bool: v, Valid: true}
	case int64:
		return sql.NullBool{Bool: v != 0, Valid: true}
	case int:
		return sql.NullBool{Bool: v != 0, Valid: true}
	case float64:
		return sql.NullBool{Bool: v != 0, Valid: true}
	case string:
		return sql.NullBool{Bool: v == "1" || strings.EqualFold(v, "true"), Valid: true}
	default:
		return sql.NullBool{}
	}
}

func updateLocalWordStatus(
	ctx context.Context,
	client *MigakuClient,
	records []wordRecord,
	update wordStatusUpdate,
	modTimestamp int64,
) error {
	if len(records) == 0 {
		return nil
	}

	query := `UPDATE WordList
SET knownStatus = ?, tracked = ?, mod = ?
WHERE dictForm = ? AND secondary = ? AND partOfSpeech = ? AND language = ?;`

	for _, record := range records {
		dictForm, secondary, partOfSpeech, language, err := requireRecordKeys(record)
		if err != nil {
			return err
		}
		if _, err := runWriteQuery(
			ctx,
			client,
			query,
			update.KnownStatus,
			update.Tracked,
			modTimestamp,
			dictForm,
			secondary,
			partOfSpeech,
			language,
		); err != nil {
			return err
		}
	}
	return nil
}

func requireRecordKeys(record wordRecord) (string, string, string, string, error) {
	if !record.DictForm.Valid || !record.PartOfSpeech.Valid || !record.Language.Valid {
		return "", "", "", "", errors.New("missing required word fields")
	}
	secondary := ""
	if record.Secondary.Valid {
		secondary = record.Secondary.String
	}
	return record.DictForm.String, secondary, record.PartOfSpeech.String, record.Language.String, nil
}

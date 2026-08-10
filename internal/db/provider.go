package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ReasoningEffort constants
const (
	ReasoningEffortNone = "none"
	ReasoningEffortLow  = "low"
	ReasoningEffortHigh = "high"
	ReasoningEffortMax  = "max"
)

// ProviderRecord represents a saved configuration record in SQLite
type ProviderRecord struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	BaseURL         string    `json:"base_url"`
	APIKey          string    `json:"api_key"`
	APIType         string    `json:"api_type"`
	Model           string    `json:"model"`
	ReasoningEffort string    `json:"reasoning_effort"`
	CustomPayload   string    `json:"custom_payload"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// CreateRecord inserts a new provider record into SQLite
func (d *DB) CreateRecord(rec *ProviderRecord) error {
	if rec.ReasoningEffort == "" {
		rec.ReasoningEffort = ReasoningEffortNone
	}
	now := time.Now()
	query := `
	INSERT INTO provider_records (name, base_url, api_key, api_type, model, reasoning_effort, custom_payload, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
	`
	res, err := d.Exec(query, rec.Name, rec.BaseURL, rec.APIKey, rec.APIType, rec.Model, rec.ReasoningEffort, rec.CustomPayload, now, now)
	if err != nil {
		return fmt.Errorf("failed to insert provider record: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	rec.ID = id
	rec.CreatedAt = now
	rec.UpdatedAt = now
	return nil
}

// ListRecords fetches all provider records ordered by updated_at DESC, id DESC
func (d *DB) ListRecords() ([]ProviderRecord, error) {
	query := `
	SELECT id, name, base_url, api_key, api_type, model, reasoning_effort, custom_payload, created_at, updated_at
	FROM provider_records
	ORDER BY updated_at DESC, id DESC;
	`
	rows, err := d.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query provider records: %w", err)
	}
	defer rows.Close()

	var records []ProviderRecord
	for rows.Next() {
		var rec ProviderRecord
		var customPayload sql.NullString
		err := rows.Scan(
			&rec.ID,
			&rec.Name,
			&rec.BaseURL,
			&rec.APIKey,
			&rec.APIType,
			&rec.Model,
			&rec.ReasoningEffort,
			&customPayload,
			&rec.CreatedAt,
			&rec.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan provider record: %w", err)
		}
		if customPayload.Valid {
			rec.CustomPayload = customPayload.String
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating provider records: %w", err)
	}
	return records, nil
}

// GetRecordByID retrieves a single provider record by ID
func (d *DB) GetRecordByID(id int64) (*ProviderRecord, error) {
	query := `
	SELECT id, name, base_url, api_key, api_type, model, reasoning_effort, custom_payload, created_at, updated_at
	FROM provider_records
	WHERE id = ?;
	`
	row := d.QueryRow(query, id)
	var rec ProviderRecord
	var customPayload sql.NullString
	err := row.Scan(
		&rec.ID,
		&rec.Name,
		&rec.BaseURL,
		&rec.APIKey,
		&rec.APIType,
		&rec.Model,
		&rec.ReasoningEffort,
		&customPayload,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query provider record by id: %w", err)
	}
	if customPayload.Valid {
		rec.CustomPayload = customPayload.String
	}
	return &rec, nil
}

// GetAPIKeyByBaseURL searches existing provider records for a matching base_url and returns saved APIKey
func (d *DB) GetAPIKeyByBaseURL(baseURL string) string {
	raw := strings.TrimSpace(baseURL)
	if raw == "" {
		return ""
	}
	normTrim := strings.TrimRight(raw, "/")
	withoutV1 := strings.TrimSuffix(normTrim, "/v1")
	withV1 := normTrim + "/v1"

	query := `
	SELECT api_key FROM provider_records
	WHERE base_url = ? OR base_url = ? OR base_url = ? OR base_url = ? OR base_url = ?
	ORDER BY updated_at DESC LIMIT 1;
	`
	var apiKey string
	err := d.QueryRow(query, raw, normTrim, normTrim+"/", withoutV1, withV1).Scan(&apiKey)
	if err == nil && apiKey != "" {
		return apiKey
	}
	return ""
}

// UpdateRecord updates an existing provider record
func (d *DB) UpdateRecord(rec *ProviderRecord) error {
	now := time.Now()
	query := `
	UPDATE provider_records
	SET name = ?, base_url = ?, api_key = ?, api_type = ?, model = ?, reasoning_effort = ?, custom_payload = ?, updated_at = ?
	WHERE id = ?;
	`
	_, err := d.Exec(query, rec.Name, rec.BaseURL, rec.APIKey, rec.APIType, rec.Model, rec.ReasoningEffort, rec.CustomPayload, now, rec.ID)
	if err != nil {
		return fmt.Errorf("failed to update provider record: %w", err)
	}
	rec.UpdatedAt = now
	return nil
}

// DeleteRecord deletes a record by ID
func (d *DB) DeleteRecord(id int64) error {
	query := `DELETE FROM provider_records WHERE id = ?;`
	_, err := d.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete provider record: %w", err)
	}
	return nil
}

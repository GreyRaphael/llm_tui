package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestProviderRecordCRUD(t *testing.T) {
	database, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	// 1. Test Create
	rec1 := &ProviderRecord{
		Name:            "DeepSeek Chat",
		BaseURL:         "https://api.deepseek.com",
		APIKey:          "sk-test123",
		APIType:         "openai_chat",
		Model:           "deepseek-chat",
		ReasoningEffort: ReasoningEffortNone,
	}

	err = database.CreateRecord(rec1)
	if err != nil {
		t.Fatalf("CreateRecord failed: %v", err)
	}
	if rec1.ID == 0 {
		t.Fatalf("expected non-zero ID after insert")
	}

	// 2. Test GetAPIKeyByBaseURL
	key := database.GetAPIKeyByBaseURL("https://api.deepseek.com")
	if key != "sk-test123" {
		t.Errorf("expected API key 'sk-test123', got '%s'", key)
	}

	// Test variant base_url match (trailing slash)
	keyVariant := database.GetAPIKeyByBaseURL("https://api.deepseek.com/")
	if keyVariant != "sk-test123" {
		t.Errorf("expected API key 'sk-test123' for variant URL, got '%s'", keyVariant)
	}

	// Test /v1 variant base_url match
	keyV1 := database.GetAPIKeyByBaseURL("https://api.deepseek.com/v1")
	if keyV1 != "sk-test123" {
		t.Errorf("expected API key 'sk-test123' for /v1 variant URL, got '%s'", keyV1)
	}

	// 3. Test List
	records, err := database.ListRecords()
	if err != nil {
		t.Fatalf("ListRecords failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Name != "DeepSeek Chat" {
		t.Errorf("expected name 'DeepSeek Chat', got '%s'", records[0].Name)
	}

	// 4. Test Create multiple for same base_url with different models & api_types
	rec2 := &ProviderRecord{
		Name:            "DeepSeek Reasoner (High)",
		BaseURL:         "https://api.deepseek.com",
		APIKey:          "sk-test123",
		APIType:         "openai_chat",
		Model:           "deepseek-reasoner",
		ReasoningEffort: ReasoningEffortHigh,
	}
	err = database.CreateRecord(rec2)
	if err != nil {
		t.Fatalf("CreateRecord rec2 failed: %v", err)
	}

	records, err = database.ListRecords()
	if err != nil {
		t.Fatalf("ListRecords after rec2 failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	// 5. Test GetByID
	fetched, err := database.GetRecordByID(rec1.ID)
	if err != nil {
		t.Fatalf("GetRecordByID failed: %v", err)
	}
	if fetched == nil || fetched.Model != "deepseek-chat" {
		t.Fatalf("GetRecordByID got wrong record: %+v", fetched)
	}

	// 6. Test Update
	fetched.Name = "DeepSeek Chat Updated"
	fetched.ReasoningEffort = ReasoningEffortLow
	err = database.UpdateRecord(fetched)
	if err != nil {
		t.Fatalf("UpdateRecord failed: %v", err)
	}

	updated, err := database.GetRecordByID(rec1.ID)
	if err != nil {
		t.Fatalf("GetRecordByID after update failed: %v", err)
	}
	if updated.Name != "DeepSeek Chat Updated" || updated.ReasoningEffort != ReasoningEffortLow {
		t.Fatalf("Update record verification failed: %+v", updated)
	}

	// 7. Test Delete
	err = database.DeleteRecord(rec1.ID)
	if err != nil {
		t.Fatalf("DeleteRecord failed: %v", err)
	}

	records, err = database.ListRecords()
	if err != nil {
		t.Fatalf("ListRecords after delete failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record after delete, got %d", len(records))
	}
}

func TestProviderRecordEmptyAPIKey(t *testing.T) {
	database, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	rec := &ProviderRecord{
		Name:            "Local No-Auth",
		BaseURL:         "http://localhost:8080/v1",
		APIKey:          "",
		APIType:         "openai_chat",
		Model:           "my-model",
		ReasoningEffort: ReasoningEffortNone,
	}
	if err := database.CreateRecord(rec); err != nil {
		t.Fatalf("CreateRecord with empty APIKey failed: %v", err)
	}

	fetched, err := database.GetRecordByID(rec.ID)
	if err != nil {
		t.Fatalf("GetRecordByID failed: %v", err)
	}
	if fetched.APIKey != "" {
		t.Errorf("expected empty APIKey, got %q", fetched.APIKey)
	}

	// GetAPIKeyByBaseURL must not conflate an empty key with "no saved record".
	// For now it returns "" for empty keys by design.
	if key := database.GetAPIKeyByBaseURL("http://localhost:8080"); key != "" {
		t.Errorf("expected empty APIKey lookup, got %q", key)
	}
}

func TestMigrateAPIKeyNotNull(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "providers.db")

	// Simulate a database created before empty API keys were supported.
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open legacy db: %v", err)
	}
	_, err = conn.Exec(`
		CREATE TABLE provider_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			base_url TEXT NOT NULL,
			api_key TEXT NOT NULL,
			api_type TEXT NOT NULL,
			model TEXT NOT NULL,
			reasoning_effort TEXT NOT NULL DEFAULT 'none',
			custom_payload TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("failed to create legacy schema: %v", err)
	}
	_, err = conn.Exec(
		`INSERT INTO provider_records (name, base_url, api_key, api_type, model) VALUES (?, ?, ?, ?, ?)`,
		"Legacy", "https://api.example.com", "sk-old", "openai_chat", "gpt-4o",
	)
	if err != nil {
		t.Fatalf("failed to seed legacy row: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("failed to close legacy conn: %v", err)
	}

	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB migration failed: %v", err)
	}
	defer database.Close()

	// Existing data should survive the migration.
	records, err := database.ListRecords()
	if err != nil {
		t.Fatalf("ListRecords after migration failed: %v", err)
	}
	if len(records) != 1 || records[0].APIKey != "sk-old" {
		t.Fatalf("legacy data lost after migration: %+v", records)
	}

	// Empty api_key should now be storable.
	if err := database.CreateRecord(&ProviderRecord{
		Name:            "New No-Auth",
		BaseURL:         "http://localhost:9000",
		APIKey:          "",
		APIType:         "openai_chat",
		Model:           "model-x",
		ReasoningEffort: ReasoningEffortNone,
	}); err != nil {
		t.Fatalf("CreateRecord with empty api_key after migration failed: %v", err)
	}

}

func TestMigrateAliasNames(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "providers.db")
	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}

	records := []*ProviderRecord{
		{Name: "gpt-4o (openai_chat)", BaseURL: "https://a.example.com", APIType: "openai_chat", Model: "gpt-4o"},
		{Name: "deepseek-v4-flash (anthropic_messages)", BaseURL: "https://b.example.com", APIType: "anthropic_messages", Model: "deepseek-v4-flash"},
		{Name: "My Custom (kept) Name", BaseURL: "https://c.example.com", APIType: "openai_responses", Model: "x"},
	}
	for _, r := range records {
		if err := database.CreateRecord(r); err != nil {
			t.Fatalf("CreateRecord failed: %v", err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	// Reopen to trigger migrations and normalize legacy aliases.
	database, err = InitDB(dbPath)
	if err != nil {
		t.Fatalf("re-init db failed: %v", err)
	}
	defer database.Close()

	got, err := database.ListRecords()
	if err != nil {
		t.Fatalf("ListRecords failed: %v", err)
	}

	names := map[string]string{}
	for _, r := range got {
		names[r.Name] = r.APIType
	}
	if names["gpt-4o"] != "openai_chat" {
		t.Errorf("expected gpt-4o alias stripped, got %v", names)
	}
	if names["deepseek-v4-flash"] != "anthropic_messages" {
		t.Errorf("expected deepseek-v4-flash alias stripped, got %v", names)
	}
	if names["My Custom (kept) Name"] != "openai_responses" {
		t.Errorf("custom name should be preserved, got %v", names)
	}
}

func TestIsDirWritable(t *testing.T) {
	tempDir := t.TempDir()
	if !isDirWritable(tempDir) {
		t.Errorf("expected tempDir %s to be writable", tempDir)
	}
	if isDirWritable("/non_existent_directory_for_testing_12345") {
		t.Errorf("expected non-existent directory not to be writable")
	}
}

func TestGetDefaultDBPath(t *testing.T) {
	dbPath := GetDefaultDBPath()
	if dbPath == "" {
		t.Fatal("expected non-empty GetDefaultDBPath")
	}
	if filepath.Base(dbPath) != "providers.db" {
		t.Errorf("expected db file name to be providers.db, got %s", filepath.Base(dbPath))
	}
}

func TestInitDBCreatesParentDir(t *testing.T) {
	tempDir := t.TempDir()
	nestedDBPath := filepath.Join(tempDir, "nested", "sub", "dir", "test_providers.db")

	database, err := InitDB(nestedDBPath)
	if err != nil {
		t.Fatalf("InitDB failed to create nested parent directories: %v", err)
	}
	defer database.Close()

	if _, err := os.Stat(nestedDBPath); os.IsNotExist(err) {
		t.Fatalf("expected database file to exist at %s", nestedDBPath)
	}
}


package db

import (
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
		APIType:         APITypeOpenAIChat,
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

	// 2. Test List
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

	// 3. Test Create multiple for same base_url with different models & api_types
	rec2 := &ProviderRecord{
		Name:            "DeepSeek Reasoner (High)",
		BaseURL:         "https://api.deepseek.com",
		APIKey:          "sk-test123",
		APIType:         APITypeOpenAIChat,
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

	// 4. Test GetByID
	fetched, err := database.GetRecordByID(rec1.ID)
	if err != nil {
		t.Fatalf("GetRecordByID failed: %v", err)
	}
	if fetched == nil || fetched.Model != "deepseek-chat" {
		t.Fatalf("GetRecordByID got wrong record: %+v", fetched)
	}

	// 5. Test Update
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

	// 6. Test Delete
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

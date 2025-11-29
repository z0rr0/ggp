package databaser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	if db == nil {
		t.Error("New() returned nil database")
	}
}

func TestNewInvalidPath(t *testing.T) {
	// Try to create database in a non-existent directory
	_, err := New("/non/existent/path/test.db")
	if err == nil {
		t.Error("New() expected error for invalid path")
	}
}

func TestInit(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.Init(ctx); err != nil {
		t.Errorf("Init() error = %v", err)
	}

	// Verify table exists
	var count int
	err = db.GetContext(ctx, &count, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='messages'")
	if err != nil {
		t.Errorf("failed to check table existence: %v", err)
	}
	if count != 1 {
		t.Errorf("messages table not created, count = %d", count)
	}
}

func TestSaveAndGetMessages(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Save some messages
	testCases := []struct {
		chatID int64
		userID int64
		text   string
	}{
		{123, 456, "Hello"},
		{123, 456, "World"},
		{789, 456, "Other chat"},
	}

	for _, tc := range testCases {
		if err := db.SaveMessage(ctx, tc.chatID, tc.userID, tc.text); err != nil {
			t.Errorf("SaveMessage() error = %v", err)
		}
	}

	// Get messages for chat 123
	messages, err := db.GetMessagesByChat(ctx, 123, 10)
	if err != nil {
		t.Errorf("GetMessagesByChat() error = %v", err)
	}

	if len(messages) != 2 {
		t.Errorf("GetMessagesByChat() returned %d messages, want 2", len(messages))
	}

	// Verify that both messages are present (order depends on ID as timestamp may be same)
	texts := make(map[string]bool)
	for _, m := range messages {
		texts[m.Text] = true
	}
	if !texts["Hello"] || !texts["World"] {
		t.Errorf("GetMessagesByChat() missing expected messages, got %v", texts)
	}
}

func TestClose(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := db.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}

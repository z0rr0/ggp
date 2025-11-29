// Package databaser provides SQLite database connection and operations.
package databaser

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite" // SQLite driver
)

// DB wraps sqlx.DB for database operations.
type DB struct {
	*sqlx.DB
}

// New creates a new database connection.
func New(path string) (*DB, error) {
	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Set connection pool settings for SQLite
	db.SetMaxOpenConns(1) // SQLite doesn't support multiple writers

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &DB{DB: db}, nil
}

// Init initializes the database schema.
func (db *DB) Init(ctx context.Context) error {
	schema := `
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			text TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_messages_chat_id ON messages(chat_id);
		CREATE INDEX IF NOT EXISTS idx_messages_user_id ON messages(user_id);
	`

	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	return nil
}

// Message represents a stored message.
type Message struct {
	ID        int64  `db:"id"`
	ChatID    int64  `db:"chat_id"`
	UserID    int64  `db:"user_id"`
	Text      string `db:"text"`
	CreatedAt string `db:"created_at"`
}

// SaveMessage stores a message in the database.
func (db *DB) SaveMessage(ctx context.Context, chatID, userID int64, text string) error {
	query := `INSERT INTO messages (chat_id, user_id, text) VALUES (?, ?, ?)`
	if _, err := db.ExecContext(ctx, query, chatID, userID, text); err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}

// GetMessagesByChat retrieves messages for a specific chat.
func (db *DB) GetMessagesByChat(ctx context.Context, chatID int64, limit int) ([]Message, error) {
	var messages []Message
	query := `SELECT id, chat_id, user_id, text, created_at FROM messages WHERE chat_id = ? ORDER BY created_at DESC LIMIT ?`
	if err := db.SelectContext(ctx, &messages, query, chatID, limit); err != nil {
		return nil, fmt.Errorf("select messages: %w", err)
	}
	return messages, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.DB.Close()
}

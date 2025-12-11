package watcher

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/z0rr0/ggp/databaser"
)

func TestBotLoggingMiddleware(t *testing.T) {
	var called bool
	next := func(_ context.Context, _ *bot.Bot, _ *models.Update) {
		called = true
	}

	middleware := BotLoggingMiddleware(next)

	tests := []struct {
		name       string
		update     *models.Update
		wantCalled bool
	}{
		{
			name: "valid message",
			update: &models.Update{
				Message: &models.Message{
					Text: "test",
					From: &models.User{},
				},
			},
			wantCalled: true,
		},
		{
			name:       "nil message",
			update:     &models.Update{},
			wantCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called = false
			ctx := context.Background()
			middleware(ctx, nil, tt.update)
			if called != tt.wantCalled {
				t.Errorf("next called = %v, want %v", called, tt.wantCalled)
			}
		})
	}
}

func TestBotAdminOnlyMiddleware(t *testing.T) {
	adminIDs := map[int64]struct{}{
		100: {},
		200: {},
	}

	// Test cases that don't involve sendErrorMessage (which requires a real bot)
	tests := []struct {
		name       string
		update     *models.Update
		wantCalled bool
	}{
		{
			name: "admin user - authorized",
			update: &models.Update{
				Message: &models.Message{
					Chat: models.Chat{ID: 123},
					From: &models.User{ID: 100, Username: "admin"},
				},
			},
			wantCalled: true,
		},
		{
			name: "second admin user - authorized",
			update: &models.Update{
				Message: &models.Message{
					Chat: models.Chat{ID: 456},
					From: &models.User{ID: 200, Username: "admin2"},
				},
			},
			wantCalled: true,
		},
		{
			name:       "nil message - rejected safely",
			update:     &models.Update{},
			wantCalled: false,
		},
		{
			name: "nil from - rejected safely",
			update: &models.Update{
				Message: &models.Message{
					Chat: models.Chat{ID: 123},
					From: nil,
				},
			},
			wantCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			next := func(_ context.Context, _ *bot.Bot, _ *models.Update) {
				called = true
			}

			middleware := BotAdminOnlyMiddleware(adminIDs)(next)
			ctx := context.Background()

			middleware(ctx, nil, tt.update)

			if called != tt.wantCalled {
				t.Errorf("next called = %v, want %v", called, tt.wantCalled)
			}
		})
	}
}

func TestBotAuthMiddleware(t *testing.T) {
	adminIDs := map[int64]struct{}{
		100: {},
	}

	// Test cases that don't involve sendErrorMessage (which requires a real bot)
	tests := []struct {
		name       string
		update     *models.Update
		setupUser  func(db *databaser.DB)
		wantCalled bool
	}{
		{
			name: "admin user - always authorized",
			update: &models.Update{
				Message: &models.Message{
					Chat: models.Chat{ID: 123},
					From: &models.User{ID: 100, Username: "admin"},
				},
			},
			wantCalled: true,
		},
		{
			name: "approved user - authorized",
			update: &models.Update{
				Message: &models.Message{
					Chat: models.Chat{ID: 456},
					From: &models.User{ID: 200, Username: "approved"},
				},
			},
			setupUser: func(db *databaser.DB) {
				ctx := context.Background()
				now := time.Now().UTC()
				_, err := db.ExecContext(ctx,
					`INSERT INTO users (id, status, username, first_name, last_name, created, updated) VALUES (?, ?, ?, '', '', ?, ?)`,
					200, 1, "approved", now, now)
				if err != nil {
					panic(err)
				}
			},
			wantCalled: true,
		},
		{
			name:       "nil message - rejected safely",
			update:     &models.Update{},
			wantCalled: false,
		},
		{
			name: "nil from - rejected safely",
			update: &models.Update{
				Message: &models.Message{
					Chat: models.Chat{ID: 123},
					From: nil,
				},
			},
			wantCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			if tt.setupUser != nil {
				tt.setupUser(db)
			}

			var called bool
			next := func(_ context.Context, _ *bot.Bot, _ *models.Update) {
				called = true
			}

			middleware := BotAuthMiddleware(adminIDs, db)(next)
			ctx := context.Background()

			middleware(ctx, nil, tt.update)

			if called != tt.wantCalled {
				t.Errorf("next called = %v, want %v", called, tt.wantCalled)
			}
		})
	}
}

func TestBotLoggingMiddleware_Duration(t *testing.T) {
	var executionStarted int64
	next := func(_ context.Context, _ *bot.Bot, _ *models.Update) {
		atomic.StoreInt64(&executionStarted, time.Now().UnixNano())
		time.Sleep(10 * time.Millisecond)
	}

	middleware := BotLoggingMiddleware(next)
	ctx := context.Background()
	update := &models.Update{
		Message: &models.Message{
			Text: "test",
			From: &models.User{},
		},
	}

	start := time.Now()
	middleware(ctx, nil, update)
	duration := time.Since(start)

	// Should have taken at least 10ms due to sleep
	if duration < 10*time.Millisecond {
		t.Errorf("middleware duration = %v, want >= 10ms", duration)
	}
}

func TestBotAdminOnlyMiddleware_EmptyAdminList(t *testing.T) {
	adminIDs := map[int64]struct{}{}

	var called bool
	next := func(_ context.Context, _ *bot.Bot, _ *models.Update) {
		called = true
	}

	middleware := BotAdminOnlyMiddleware(adminIDs)(next)

	// With empty admin list and nil message, middleware should return early without calling next
	// (nil message test avoids the sendErrorMessage that would panic with nil bot)
	ctx := context.Background()
	update := &models.Update{Message: nil}

	middleware(ctx, nil, update)

	if called {
		t.Error("next should not be called with nil message")
	}
}

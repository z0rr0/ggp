package watcher

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/z0rr0/ggp/databaser"
)

func seedUser(t *testing.T, db *databaser.DB, id int64, status uint8, username string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, status, username, first_name, last_name, created, updated) VALUES (?, ?, ?, 'First', 'Last', ?, ?)`,
		id, status, username, now, now)
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}
}

func TestHandleUsers(t *testing.T) {
	tests := []struct {
		name            string
		setupUsers      func(db *databaser.DB, t *testing.T)
		wantMsgCalls    int
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:         "empty users list",
			wantMsgCalls: 1,
			wantContains: []string{"Пользователи:"},
		},
		{
			name: "multiple users with different statuses",
			setupUsers: func(db *databaser.DB, t *testing.T) {
				seedUser(t, db, 100, 0, "pending_user")
				seedUser(t, db, 200, 1, "approved_user")
				seedUser(t, db, 300, 2, "rejected_user")
			},
			wantMsgCalls: 1,
			wantContains: []string{
				"Пользователи:",
				"@pending_user",
				"@approved_user",
				"@rejected_user",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			if tt.setupUsers != nil {
				tt.setupUsers(db, t)
			}
			cfg := newTestConfig(456)
			handler := NewBotHandler(db, cfg, nil)
			mBot := &mockBot{}
			ctx := context.Background()

			update := &models.Update{
				Message: &models.Message{
					Chat: models.Chat{ID: 123},
					From: &models.User{ID: 456},
					Text: "/users",
				},
			}

			handler.HandleUsers(ctx, mBot, update)

			if mBot.sendMessageCalls != tt.wantMsgCalls {
				t.Errorf("SendMessage called %d times, want %d", mBot.sendMessageCalls, tt.wantMsgCalls)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(mBot.lastText, want) {
					t.Errorf("response should contain %q, got: %s", want, mBot.lastText)
				}
			}
		})
	}
}
func TestHandleUsers_DatabaseError(t *testing.T) {
	db := newTestDB(t)
	cfg := newTestConfig(456)
	handler := NewBotHandler(db, cfg, nil)
	mBot := &mockBot{}
	ctx := context.Background()

	if err := db.Close(); err != nil {
		t.Fatalf("failed to close db: %v", err)
	}

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 123},
			From: &models.User{ID: 456},
		},
	}

	handler.HandleUsers(ctx, mBot, update)

	if mBot.sendMessageCalls != 1 {
		t.Errorf("SendMessage called %d times, want 1 (error message)", mBot.sendMessageCalls)
	}
}

func TestHandleApprove(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		setupUser    func(db *databaser.DB, t *testing.T)
		wantMsgCalls int
		wantApproved bool
		wantError    bool
	}{
		{
			name: "approve pending user",
			text: "/approve 100",
			setupUser: func(db *databaser.DB, t *testing.T) {
				seedUser(t, db, 100, 0, "pending")
			},
			wantMsgCalls: 2, // confirmation + notification to user
			wantApproved: true,
		},
		{
			name:         "missing user_id argument",
			text:         "/approve",
			wantMsgCalls: 1,
			wantError:    true,
		},
		{
			name:         "invalid user_id format",
			text:         "/approve abc",
			wantMsgCalls: 1,
			wantError:    true,
		},
		{
			name:         "approve non-existent user",
			text:         "/approve 999",
			wantMsgCalls: 1,
			wantError:    true,
		},
		{
			name: "approve already approved user",
			text: "/approve 200",
			setupUser: func(db *databaser.DB, t *testing.T) {
				seedUser(t, db, 200, 1, "approved")
			},
			wantMsgCalls: 1,
			wantError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			if tt.setupUser != nil {
				tt.setupUser(db, t)
			}
			cfg := newTestConfig(456)
			handler := NewBotHandler(db, cfg, nil)
			mBot := &mockBot{}
			ctx := context.Background()

			update := &models.Update{
				Message: &models.Message{
					Chat: models.Chat{ID: 123},
					From: &models.User{ID: 456},
					Text: tt.text,
				},
			}

			handler.HandleApprove(ctx, mBot, update)

			if mBot.sendMessageCalls != tt.wantMsgCalls {
				t.Errorf("SendMessage called %d times, want %d", mBot.sendMessageCalls, tt.wantMsgCalls)
			}

			if tt.wantApproved {
				user, err := db.GetUser(ctx, 100)
				if err != nil {
					t.Fatalf("GetUser() error = %v", err)
				}
				if !user.IsApproved() {
					t.Errorf("user status = %d, want approved", user.Status)
				}
			}
		})
	}
}

func TestHandleApprove_SendError(t *testing.T) {
	db := newTestDB(t)
	seedUser(t, db, 100, 0, "pending")
	cfg := newTestConfig(456)
	handler := NewBotHandler(db, cfg, nil)
	mBot := &mockBot{sendMessageErr: errors.New("send error")}
	ctx := context.Background()

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 123},
			From: &models.User{ID: 456},
			Text: "/approve 100",
		},
	}

	handler.HandleApprove(ctx, mBot, update)

	// Should still try to send messages even with errors
	if mBot.sendMessageCalls < 1 {
		t.Errorf("SendMessage should be called at least once")
	}
}

func TestHandleReject(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		setupUser    func(db *databaser.DB, t *testing.T)
		wantMsgCalls int
		wantRejected bool
		wantError    bool
	}{
		{
			name: "reject pending user",
			text: "/reject 100",
			setupUser: func(db *databaser.DB, t *testing.T) {
				seedUser(t, db, 100, 0, "pending")
			},
			wantMsgCalls: 2, // confirmation + notification to user
			wantRejected: true,
		},
		{
			name: "reject approved user",
			text: "/reject 200",
			setupUser: func(db *databaser.DB, t *testing.T) {
				seedUser(t, db, 200, 1, "approved")
			},
			wantMsgCalls: 2,
			wantRejected: true,
		},
		{
			name:         "missing user_id argument",
			text:         "/reject",
			wantMsgCalls: 1,
			wantError:    true,
		},
		{
			name:         "invalid user_id format",
			text:         "/reject xyz",
			wantMsgCalls: 1,
			wantError:    true,
		},
		{
			name:         "reject non-existent user",
			text:         "/reject 999",
			wantMsgCalls: 1,
			wantError:    true,
		},
		{
			name: "reject already rejected user",
			text: "/reject 300",
			setupUser: func(db *databaser.DB, t *testing.T) {
				seedUser(t, db, 300, 2, "rejected")
			},
			wantMsgCalls: 1,
			wantError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			if tt.setupUser != nil {
				tt.setupUser(db, t)
			}
			cfg := newTestConfig(456)
			handler := NewBotHandler(db, cfg, nil)
			mBot := &mockBot{}
			ctx := context.Background()

			update := &models.Update{
				Message: &models.Message{
					Chat: models.Chat{ID: 123},
					From: &models.User{ID: 456},
					Text: tt.text,
				},
			}

			handler.HandleReject(ctx, mBot, update)

			if mBot.sendMessageCalls != tt.wantMsgCalls {
				t.Errorf("SendMessage called %d times, want %d", mBot.sendMessageCalls, tt.wantMsgCalls)
			}

			if tt.wantRejected {
				var userID int64
				if tt.text == "/reject 100" {
					userID = 100
				} else if tt.text == "/reject 200" {
					userID = 200
				}
				if userID > 0 {
					user, err := db.GetUser(ctx, userID)
					if err != nil {
						t.Fatalf("GetUser() error = %v", err)
					}
					if !user.IsRejected() {
						t.Errorf("user status = %d, want rejected", user.Status)
					}
				}
			}
		})
	}
}

func TestHandleReject_SendError(t *testing.T) {
	db := newTestDB(t)
	seedUser(t, db, 100, 0, "pending")
	cfg := newTestConfig(456)
	handler := NewBotHandler(db, cfg, nil)
	mBot := &mockBot{sendMessageErr: errors.New("send error")}
	ctx := context.Background()

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 123},
			From: &models.User{ID: 456},
			Text: "/reject 100",
		},
	}

	handler.HandleReject(ctx, mBot, update)

	if mBot.sendMessageCalls < 1 {
		t.Errorf("SendMessage should be called at least once")
	}
}

func TestHandleUsers_StatusSymbols(t *testing.T) {
	db := newTestDB(t)
	seedUser(t, db, 100, 0, "pending_user")
	seedUser(t, db, 200, 1, "approved_user")
	seedUser(t, db, 300, 2, "rejected_user")

	cfg := newTestConfig(456)
	handler := NewBotHandler(db, cfg, nil)
	mBot := &mockBot{}
	ctx := context.Background()

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 123},
			From: &models.User{ID: 456},
		},
	}

	handler.HandleUsers(ctx, mBot, update)

	// Check status symbols are present
	if !strings.Contains(mBot.lastText, "⏳") {
		t.Error("response should contain pending symbol ⏳")
	}
	if !strings.Contains(mBot.lastText, "✅") {
		t.Error("response should contain approved symbol ✅")
	}
	if !strings.Contains(mBot.lastText, "❌") {
		t.Error("response should contain rejected symbol ❌")
	}
}

func TestHandleApprove_NotifiesUser(t *testing.T) {
	db := newTestDB(t)
	seedUser(t, db, 100, 0, "pending")
	cfg := newTestConfig(456)
	handler := NewBotHandler(db, cfg, nil)
	mBot := &mockBot{}
	ctx := context.Background()

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 456}, // admin chat
			From: &models.User{ID: 456},
			Text: "/approve 100",
		},
	}

	handler.HandleApprove(ctx, mBot, update)

	// Should send 2 messages: one to admin, one to user
	if mBot.sendMessageCalls != 2 {
		t.Errorf("SendMessage called %d times, want 2", mBot.sendMessageCalls)
	}

	// Last message should be to the approved user
	if chatID, ok := mBot.lastChatID.(int64); ok && chatID != 100 {
		t.Errorf("last message sent to chat %d, want 100 (approved user)", chatID)
	}
}

func TestHandleReject_NotifiesUser(t *testing.T) {
	db := newTestDB(t)
	seedUser(t, db, 100, 0, "pending")
	cfg := newTestConfig(456)
	handler := NewBotHandler(db, cfg, nil)
	mBot := &mockBot{}
	ctx := context.Background()

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 456}, // admin chat
			From: &models.User{ID: 456},
			Text: "/reject 100",
		},
	}

	handler.HandleReject(ctx, mBot, update)

	// Should send 2 messages: one to admin, one to user
	if mBot.sendMessageCalls != 2 {
		t.Errorf("SendMessage called %d times, want 2", mBot.sendMessageCalls)
	}

	// Last message should be to the rejected user
	if chatID, ok := mBot.lastChatID.(int64); ok && chatID != 100 {
		t.Errorf("last message sent to chat %d, want 100 (rejected user)", chatID)
	}
}

func TestHandleApprove_WithExtraArgs(t *testing.T) {
	db := newTestDB(t)
	seedUser(t, db, 100, 0, "pending")
	cfg := newTestConfig(456)
	handler := NewBotHandler(db, cfg, nil)
	mBot := &mockBot{}
	ctx := context.Background()

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 123},
			From: &models.User{ID: 456},
			Text: "/approve 100 extra args ignored",
		},
	}

	handler.HandleApprove(ctx, mBot, update)

	// Should still work, extra args are ignored
	if mBot.sendMessageCalls != 2 {
		t.Errorf("SendMessage called %d times, want 2", mBot.sendMessageCalls)
	}
}

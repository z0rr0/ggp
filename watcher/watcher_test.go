package watcher

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/z0rr0/ggp/config"
	"github.com/z0rr0/ggp/databaser"
	"github.com/z0rr0/ggp/predictor"
)

type mockBot struct {
	sendMessageCalls int
	sendPhotoCalls   int
	lastChatID       any
	lastText         string
	lastCaption      string
	sendMessageErr   error
	sendPhotoErr     error
}

func (m *mockBot) SendMessage(_ context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	m.sendMessageCalls++
	m.lastChatID = params.ChatID
	m.lastText = params.Text
	return &models.Message{}, m.sendMessageErr
}

func (m *mockBot) SendPhoto(_ context.Context, params *bot.SendPhotoParams) (*models.Message, error) {
	m.sendPhotoCalls++
	m.lastChatID = params.ChatID
	m.lastCaption = params.Caption
	return &models.Message{}, m.sendPhotoErr
}

func newTestDB(t *testing.T) *databaser.DB {
	t.Helper()
	ctx := context.Background()
	db, err := databaser.New(ctx, ":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close test database: %v", err)
		}
	})
	return db
}

func newTestConfig(adminIDs ...int64) *config.Config {
	cfg := &config.Config{
		Base: config.Base{
			TimeLocation: time.UTC,
			AdminIDs:     make(map[int64]struct{}),
			Admins:       adminIDs,
		},
	}
	for _, id := range adminIDs {
		cfg.Base.AdminIDs[id] = struct{}{}
	}
	return cfg
}

func newTestController(t *testing.T, db *databaser.DB) *predictor.Controller {
	t.Helper()
	cfg := &config.Config{
		Base: config.Base{
			TimeLocation: time.UTC,
		},
		Predictor: config.Predictor{
			Hours: 6,
		},
	}
	ctx := context.Background()
	ctrl, err := predictor.Run(ctx, db, nil, cfg)
	if err != nil {
		t.Fatalf("failed to create predictor controller: %v", err)
	}
	return ctrl
}

func seedEvents(t *testing.T, db *databaser.DB, count int) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	events := make([]databaser.Event, count)
	for i := 0; i < count; i++ {
		events[i] = databaser.Event{
			Timestamp: now.Add(-time.Duration(count-i) * time.Hour),
			Load:      uint8(50 + i%50),
		}
	}
	if err := db.SaveManyEvents(ctx, events); err != nil {
		t.Fatalf("failed to seed events: %v", err)
	}
}

func TestNewBotHandler(t *testing.T) {
	db := newTestDB(t)
	cfg := newTestConfig(123)
	pc := newTestController(t, db)

	handler := NewBotHandler(db, cfg, pc)
	if handler == nil {
		t.Fatal("NewBotHandler returned nil")
	}
	if handler.db != db {
		t.Error("db not set correctly")
	}
	if handler.cfg != cfg {
		t.Error("cfg not set correctly")
	}
	if handler.pc != pc {
		t.Error("pc not set correctly")
	}
}

func TestHandleStart(t *testing.T) {
	tests := []struct {
		name          string
		update        *models.Update
		wantErr       bool
		wantCalls     int
		expectWarning bool
	}{
		{
			name: "valid message",
			update: &models.Update{
				Message: &models.Message{
					Chat: models.Chat{ID: 123},
					From: &models.User{ID: 456},
					Text: CmdStart,
				},
			},
			wantCalls: 1,
		},
		{
			name:          "nil message",
			update:        &models.Update{},
			wantCalls:     0,
			expectWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			cfg := newTestConfig(456)
			handler := NewBotHandler(db, cfg, nil)
			mBot := &mockBot{}
			ctx := context.Background()

			handler.HandleStart(ctx, mBot, tt.update)

			if mBot.sendMessageCalls != tt.wantCalls {
				t.Errorf("SendMessage called %d times, want %d", mBot.sendMessageCalls, tt.wantCalls)
			}

			if tt.wantCalls > 0 {
				if chatID, ok := mBot.lastChatID.(int64); ok && chatID != 123 {
					t.Errorf("lastChatID = %d, want 123", chatID)
				}
			}
		})
	}
}

func TestHandleStart_SendError(t *testing.T) {
	db := newTestDB(t)
	cfg := newTestConfig(456)
	handler := NewBotHandler(db, cfg, nil)
	mBot := &mockBot{sendMessageErr: errors.New("send error")}
	ctx := context.Background()

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 123},
			From: &models.User{ID: 456},
		},
	}

	handler.HandleStart(ctx, mBot, update)

	if mBot.sendMessageCalls != 1 {
		t.Errorf("SendMessage called %d times, want 1", mBot.sendMessageCalls)
	}
}

func TestHandleWeek(t *testing.T) {
	db := newTestDB(t)
	seedEvents(t, db, 10)
	cfg := newTestConfig(456)
	pc := newTestController(t, db)
	handler := NewBotHandler(db, cfg, pc)
	mBot := &mockBot{}
	ctx := context.Background()

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 123},
			From: &models.User{ID: 456},
			Text: CmdWeek,
		},
	}

	handler.HandleWeek(ctx, mBot, update)

	if mBot.sendPhotoCalls != 1 {
		t.Errorf("SendPhoto called %d times, want 1", mBot.sendPhotoCalls)
	}
}

func TestHandleDay(t *testing.T) {
	db := newTestDB(t)
	seedEvents(t, db, 10)
	cfg := newTestConfig(456)
	pc := newTestController(t, db)
	handler := NewBotHandler(db, cfg, pc)
	mBot := &mockBot{}
	ctx := context.Background()

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 123},
			From: &models.User{ID: 456},
			Text: CmdDay,
		},
	}

	handler.HandleDay(ctx, mBot, update)

	if mBot.sendPhotoCalls != 1 {
		t.Errorf("SendPhoto called %d times, want 1", mBot.sendPhotoCalls)
	}
}

func TestHandleHalfDay(t *testing.T) {
	db := newTestDB(t)
	seedEvents(t, db, 10)
	cfg := newTestConfig(456)
	pc := newTestController(t, db)
	handler := NewBotHandler(db, cfg, pc)
	mBot := &mockBot{}
	ctx := context.Background()

	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 123},
			From: &models.User{ID: 456},
			Text: CmdHalfDay,
		},
	}

	handler.HandleHalfDay(ctx, mBot, update)

	if mBot.sendPhotoCalls != 1 {
		t.Errorf("SendPhoto called %d times, want 1", mBot.sendPhotoCalls)
	}
}

func TestDefaultHandler(t *testing.T) {
	tests := []struct {
		name           string
		userID         int64
		text           string
		wantPhotoCalls int
		wantMsgCalls   int
	}{
		{
			name:           "valid duration - authorized user",
			userID:         456,
			text:           "6h",
			wantPhotoCalls: 1,
			wantMsgCalls:   0,
		},
		{
			name:           "invalid duration - authorized user",
			userID:         456,
			text:           "invalid",
			wantPhotoCalls: 0,
			wantMsgCalls:   1,
		},
		{
			name:           "unauthorized user",
			userID:         999,
			text:           "6h",
			wantPhotoCalls: 0,
			wantMsgCalls:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			seedEvents(t, db, 10)
			cfg := newTestConfig(456)
			pc := newTestController(t, db)
			handler := NewBotHandler(db, cfg, pc)
			mBot := &mockBot{}
			ctx := context.Background()

			update := &models.Update{
				Message: &models.Message{
					Chat: models.Chat{ID: 123},
					From: &models.User{ID: tt.userID},
					Text: tt.text,
				},
			}

			handler.DefaultHandler(ctx, mBot, update)

			if mBot.sendPhotoCalls != tt.wantPhotoCalls {
				t.Errorf("SendPhoto called %d times, want %d", mBot.sendPhotoCalls, tt.wantPhotoCalls)
			}
			if mBot.sendMessageCalls != tt.wantMsgCalls {
				t.Errorf("SendMessage called %d times, want %d", mBot.sendMessageCalls, tt.wantMsgCalls)
			}
		})
	}
}

func TestDefaultHandler_NilMessage(t *testing.T) {
	db := newTestDB(t)
	cfg := newTestConfig(456)
	handler := NewBotHandler(db, cfg, nil)
	mBot := &mockBot{}
	ctx := context.Background()

	update := &models.Update{}

	handler.DefaultHandler(ctx, mBot, update)

	if mBot.sendPhotoCalls != 0 {
		t.Errorf("SendPhoto called %d times, want 0", mBot.sendPhotoCalls)
	}
}

func TestHandlePeriod(t *testing.T) {
	tests := []struct {
		name           string
		update         *models.Update
		userID         int64
		wantPhotoCalls int
	}{
		{
			name: "authorized user",
			update: &models.Update{
				Message: &models.Message{
					Chat: models.Chat{ID: 123},
					From: &models.User{ID: 456},
					Text: "test",
				},
			},
			wantPhotoCalls: 1,
		},
		{
			name: "unauthorized user",
			update: &models.Update{
				Message: &models.Message{
					Chat: models.Chat{ID: 123},
					From: &models.User{ID: 999},
					Text: "test",
				},
			},
			wantPhotoCalls: 0,
		},
		{
			name: "nil message",
			update: &models.Update{
				Message: nil,
			},
			wantPhotoCalls: 0,
		},
		{
			name: "nil from",
			update: &models.Update{
				Message: &models.Message{
					Chat: models.Chat{ID: 123},
					From: nil,
				},
			},
			wantPhotoCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			seedEvents(t, db, 10)
			cfg := newTestConfig(456)
			pc := newTestController(t, db)
			handler := NewBotHandler(db, cfg, pc)
			mBot := &mockBot{}
			ctx := context.Background()

			handler.handlePeriod(ctx, mBot, tt.update, 24*time.Hour, 6)

			if mBot.sendPhotoCalls != tt.wantPhotoCalls {
				t.Errorf("SendPhoto called %d times, want %d", mBot.sendPhotoCalls, tt.wantPhotoCalls)
			}
		})
	}
}

func TestIsAuthorized(t *testing.T) {
	tests := []struct {
		name     string
		adminIDs []int64
		userID   int64
		want     bool
	}{
		{
			name:     "authorized user",
			adminIDs: []int64{123, 456, 789},
			userID:   456,
			want:     true,
		},
		{
			name:     "unauthorized user",
			adminIDs: []int64{123, 456},
			userID:   999,
			want:     false,
		},
		{
			name:     "empty admin list",
			adminIDs: []int64{},
			userID:   123,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			cfg := newTestConfig(tt.adminIDs...)
			handler := NewBotHandler(db, cfg, nil)

			got := handler.isAuthorized(tt.userID)
			if got != tt.want {
				t.Errorf("isAuthorized(%d) = %v, want %v", tt.userID, got, tt.want)
			}
		})
	}
}

func TestCalculatePredictHours(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     uint8
	}{
		{duration: 30 * time.Minute, want: 1},
		{duration: time.Hour, want: 1},
		{duration: 2 * time.Hour, want: 2},
		{duration: 4 * time.Hour, want: 2},
		{duration: 6 * time.Hour, want: 4},
		{duration: 12 * time.Hour, want: 4},
		{duration: 18 * time.Hour, want: 6},
		{duration: 24 * time.Hour, want: 6},
		{duration: 48 * time.Hour, want: 12},
		{duration: 7 * 24 * time.Hour, want: 12},
	}

	for _, tt := range tests {
		t.Run(tt.duration.String(), func(t *testing.T) {
			got := calculatePredictHours(tt.duration)
			if got != tt.want {
				t.Errorf("calculatePredictHours(%v) = %d, want %d", tt.duration, got, tt.want)
			}
		})
	}
}

func TestBuildGraph(t *testing.T) {
	tests := []struct {
		name             string
		eventsCount      int
		wantPhotoCalls   int
		wantMessageCalls int
	}{
		{
			name:             "sufficient events",
			eventsCount:      10,
			wantPhotoCalls:   1,
			wantMessageCalls: 0,
		},
		{
			name:             "exactly 2 events",
			eventsCount:      2,
			wantPhotoCalls:   1,
			wantMessageCalls: 0,
		},
		{
			name:             "insufficient events - 1",
			eventsCount:      1,
			wantPhotoCalls:   0,
			wantMessageCalls: 1,
		},
		{
			name:             "no events",
			eventsCount:      0,
			wantPhotoCalls:   0,
			wantMessageCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			if tt.eventsCount > 0 {
				seedEvents(t, db, tt.eventsCount)
			}
			cfg := newTestConfig(456)
			pc := newTestController(t, db)
			handler := NewBotHandler(db, cfg, pc)
			mBot := &mockBot{}
			ctx := context.Background()

			handler.buildGraph(ctx, mBot, 123, 24*time.Hour, 6)

			if mBot.sendPhotoCalls != tt.wantPhotoCalls {
				t.Errorf("SendPhoto called %d times, want %d", mBot.sendPhotoCalls, tt.wantPhotoCalls)
			}
			if mBot.sendMessageCalls != tt.wantMessageCalls {
				t.Errorf("SendMessage called %d times, want %d", mBot.sendMessageCalls, tt.wantMessageCalls)
			}
		})
	}
}

func TestBuildGraph_WithoutPredictor(t *testing.T) {
	db := newTestDB(t)
	seedEvents(t, db, 10)
	cfg := newTestConfig(456)
	handler := NewBotHandler(db, cfg, nil)
	mBot := &mockBot{}
	ctx := context.Background()

	handler.buildGraph(ctx, mBot, 123, 24*time.Hour, 6)

	if mBot.sendPhotoCalls != 1 {
		t.Errorf("SendPhoto called %d times, want 1", mBot.sendPhotoCalls)
	}
}

func TestBuildGraph_SendPhotoError(t *testing.T) {
	db := newTestDB(t)
	seedEvents(t, db, 10)
	cfg := newTestConfig(456)
	pc := newTestController(t, db)
	handler := NewBotHandler(db, cfg, pc)
	mBot := &mockBot{sendPhotoErr: errors.New("photo error")}
	ctx := context.Background()

	handler.buildGraph(ctx, mBot, 123, 24*time.Hour, 6)

	if mBot.sendPhotoCalls != 1 {
		t.Errorf("SendPhoto called %d times, want 1", mBot.sendPhotoCalls)
	}
	if mBot.sendMessageCalls != 1 {
		t.Errorf("SendMessage called %d times, want 1 (error message)", mBot.sendMessageCalls)
	}
}

func TestSendErrorMessage(t *testing.T) {
	tests := []struct {
		name             string
		err              error
		sendMessageErr   error
		wantMessageCalls int
	}{
		{
			name:             "error with successful send",
			err:              errors.New("test error"),
			wantMessageCalls: 1,
		},
		{
			name:             "no error with successful send",
			err:              nil,
			wantMessageCalls: 1,
		},
		{
			name:             "error with failed send",
			err:              errors.New("test error"),
			sendMessageErr:   errors.New("send error"),
			wantMessageCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mBot := &mockBot{sendMessageErr: tt.sendMessageErr}
			ctx := context.Background()

			sendErrorMessage(ctx, tt.err, mBot, 123, "test message")

			if mBot.sendMessageCalls != tt.wantMessageCalls {
				t.Errorf("SendMessage called %d times, want %d", mBot.sendMessageCalls, tt.wantMessageCalls)
			}
			if mBot.lastText != "test message" {
				t.Errorf("lastText = %q, want %q", mBot.lastText, "test message")
			}
		})
	}
}

func TestBuildGraph_DatabaseError(t *testing.T) {
	db := newTestDB(t)
	cfg := newTestConfig(456)
	handler := NewBotHandler(db, cfg, nil)
	mBot := &mockBot{}
	ctx := context.Background()

	if err := db.Close(); err != nil {
		t.Fatalf("failed to close db: %v", err)
	}

	handler.buildGraph(ctx, mBot, 123, 24*time.Hour, 6)

	if mBot.sendMessageCalls != 1 {
		t.Errorf("SendMessage called %d times, want 1 (error message)", mBot.sendMessageCalls)
	}
}

func TestBuildGraph_CaptionFormat(t *testing.T) {
	db := newTestDB(t)
	seedEvents(t, db, 3)
	cfg := newTestConfig(456)
	handler := NewBotHandler(db, cfg, nil)
	mBot := &mockBot{}
	ctx := context.Background()

	handler.buildGraph(ctx, mBot, 123, 24*time.Hour, 6)

	if mBot.lastCaption == "" {
		t.Error("caption is empty")
	}
}

// Ensure mockBot implements BotAPI interface
var _ BotAPI = (*mockBot)(nil)

// benchmarkBot provides a minimal bot implementation for benchmarks
type benchmarkBot struct{}

func (b *benchmarkBot) SendMessage(_ context.Context, _ *bot.SendMessageParams) (*models.Message, error) {
	return &models.Message{}, nil
}

func (b *benchmarkBot) SendPhoto(_ context.Context, params *bot.SendPhotoParams) (*models.Message, error) {
	if params.Photo != nil {
		if upload, ok := params.Photo.(*models.InputFileUpload); ok && upload.Data != nil {
			_, err := io.Copy(io.Discard, upload.Data)
			if err != nil {
				return nil, err
			}
		}
	}
	return &models.Message{}, nil
}

// Ensure benchmarkBot implements BotAPI interface
var _ BotAPI = (*benchmarkBot)(nil)

func BenchmarkHandleStart(b *testing.B) {
	ctx := context.Background()
	db := newTestDB(&testing.T{})
	defer func() {
		if err := db.Close(); err != nil {
			b.Errorf("failed to close db: %v", err)
		}
	}()
	cfg := newTestConfig(456)
	handler := NewBotHandler(db, cfg, nil)
	bBot := &benchmarkBot{}
	update := &models.Update{
		Message: &models.Message{
			Chat: models.Chat{ID: 123},
			From: &models.User{ID: 456},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler.HandleStart(ctx, bBot, update)
	}
}

func BenchmarkBuildGraph(b *testing.B) {
	ctx := context.Background()
	db := newTestDB(&testing.T{})
	defer func() {
		if err := db.Close(); err != nil {
			b.Errorf("failed to close db: %v", err)
		}
	}()
	seedEvents(&testing.T{}, db, 100)
	cfg := newTestConfig(456)
	pc := newTestController(&testing.T{}, db)
	handler := NewBotHandler(db, cfg, pc)
	bBot := &benchmarkBot{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler.buildGraph(ctx, bBot, 123, 24*time.Hour, 6)
	}
}

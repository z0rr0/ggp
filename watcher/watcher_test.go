package watcher

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/z0rr0/ggp/config"
	"github.com/z0rr0/ggp/databaser"
)

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

func newTestConfig(t *testing.T, adminIDs map[int64]struct{}) *config.Config {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}
	return &config.Config{
		Base: config.Base{
			TimeLocation: loc,
			AdminIDs:     adminIDs,
		},
	}
}

// mockBot implements a minimal mock for testing bot interactions.
type mockBot struct {
	mu            sync.Mutex
	sentMessages  []bot.SendMessageParams
	sentPhotos    []bot.SendPhotoParams
	answeredCBs   []bot.AnswerCallbackQueryParams
	sendMsgErr    error
	sendPhotoErr  error
	answerCBErr   error
	sendMsgResult *models.Message
	sendPhotoMsg  *models.Message
}

func (m *mockBot) SendMessage(_ context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentMessages = append(m.sentMessages, *params)
	if m.sendMsgErr != nil {
		return nil, m.sendMsgErr
	}
	if m.sendMsgResult != nil {
		return m.sendMsgResult, nil
	}
	chatID, _ := params.ChatID.(int64)
	return &models.Message{ID: 1, Chat: models.Chat{ID: chatID}}, nil
}

func (m *mockBot) SendPhoto(_ context.Context, params *bot.SendPhotoParams) (*models.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentPhotos = append(m.sentPhotos, *params)
	if m.sendPhotoErr != nil {
		return nil, m.sendPhotoErr
	}
	if m.sendPhotoMsg != nil {
		return m.sendPhotoMsg, nil
	}
	chatID, _ := params.ChatID.(int64)
	return &models.Message{ID: 1, Chat: models.Chat{ID: chatID}}, nil
}

func (m *mockBot) AnswerCallbackQuery(_ context.Context, params *bot.AnswerCallbackQueryParams) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.answeredCBs = append(m.answeredCBs, *params)
	if m.answerCBErr != nil {
		return false, m.answerCBErr
	}
	return true, nil
}

func (m *mockBot) getSentMessages() []bot.SendMessageParams {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]bot.SendMessageParams, len(m.sentMessages))
	copy(result, m.sentMessages)
	return result
}

func (m *mockBot) getSentPhotos() []bot.SendPhotoParams {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]bot.SendPhotoParams, len(m.sentPhotos))
	copy(result, m.sentPhotos)
	return result
}

func (m *mockBot) getAnsweredCBs() []bot.AnswerCallbackQueryParams {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]bot.AnswerCallbackQueryParams, len(m.answeredCBs))
	copy(result, m.answeredCBs)
	return result
}

func TestNewBotHandler(t *testing.T) {
	db := newTestDB(t)
	cfg := newTestConfig(t, nil)

	tests := []struct {
		name string
		db   *databaser.DB
		cfg  *config.Config
	}{
		{
			name: "with all dependencies",
			db:   db,
			cfg:  cfg,
		},
		{
			name: "with nil predictor controller",
			db:   db,
			cfg:  cfg,
		},
		{
			name: "with nil db",
			db:   nil,
			cfg:  cfg,
		},
		{
			name: "with nil config",
			db:   db,
			cfg:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewBotHandler(tt.db, tt.cfg, nil)
			if h == nil {
				t.Error("NewBotHandler() returned nil")
			}
			if h.db != tt.db {
				t.Errorf("NewBotHandler() db = %v, want %v", h.db, tt.db)
			}
			if h.cfg != tt.cfg {
				t.Errorf("NewBotHandler() cfg = %v, want %v", h.cfg, tt.cfg)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		want     string
	}{
		{name: "StartCommand", constant: StartCommand, want: "/start"},
		{name: "CallbackPrefix", constant: CallbackPrefix, want: "/period"},
		{name: "MenuCommand", constant: MenuCommand, want: "📋 Меню"},
		{name: "callbackHalfDay", constant: callbackHalfDay, want: "/periodHalfDay"},
		{name: "callbackDay", constant: callbackDay, want: "/periodDay"},
		{name: "callbackWeek", constant: callbackWeek, want: "/periodWeek"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.want {
				t.Errorf("constant %s = %q, want %q", tt.name, tt.constant, tt.want)
			}
		})
	}
}

func TestCallbackPeriodMapping(t *testing.T) {
	tests := []struct {
		name         string
		callback     string
		wantDuration time.Duration
		wantPredict  uint8
		wantSkip     bool
	}{
		{
			name:         "half day callback",
			callback:     callbackHalfDay,
			wantDuration: 12 * time.Hour,
			wantPredict:  4,
			wantSkip:     false,
		},
		{
			name:         "day callback",
			callback:     callbackDay,
			wantDuration: 24 * time.Hour,
			wantPredict:  6,
			wantSkip:     false,
		},
		{
			name:         "week callback",
			callback:     callbackWeek,
			wantDuration: 7 * 24 * time.Hour,
			wantPredict:  12,
			wantSkip:     false,
		},
		{
			name:     "unknown callback",
			callback: "/unknown",
			wantSkip: true,
		},
		{
			name:     "empty callback",
			callback: "",
			wantSkip: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				duration     time.Duration
				predictHours uint8
				skip         bool
			)

			switch tt.callback {
			case callbackHalfDay:
				duration = 12 * time.Hour
				predictHours = 4
			case callbackDay:
				duration = 24 * time.Hour
				predictHours = 6
			case callbackWeek:
				duration = 7 * 24 * time.Hour
				predictHours = 12
			default:
				skip = true
			}

			if skip != tt.wantSkip {
				t.Errorf("skip = %v, wantSkip = %v", skip, tt.wantSkip)
			}
			if !tt.wantSkip {
				if duration != tt.wantDuration {
					t.Errorf("duration = %v, want %v", duration, tt.wantDuration)
				}
				if predictHours != tt.wantPredict {
					t.Errorf("predictHours = %d, want %d", predictHours, tt.wantPredict)
				}
			}
		})
	}
}

func TestDefaultHandlerPredictHoursMapping(t *testing.T) {
	tests := []struct {
		name        string
		duration    time.Duration
		wantPredict uint8
	}{
		{name: "30 minutes", duration: 30 * time.Minute, wantPredict: 1},
		{name: "1 hour exactly", duration: time.Hour, wantPredict: 1},
		{name: "2 hours", duration: 2 * time.Hour, wantPredict: 2},
		{name: "4 hours exactly", duration: 4 * time.Hour, wantPredict: 2},
		{name: "6 hours", duration: 6 * time.Hour, wantPredict: 4},
		{name: "12 hours exactly", duration: 12 * time.Hour, wantPredict: 4},
		{name: "18 hours", duration: 18 * time.Hour, wantPredict: 6},
		{name: "24 hours exactly", duration: 24 * time.Hour, wantPredict: 6},
		{name: "48 hours", duration: 48 * time.Hour, wantPredict: 12},
		{name: "1 week", duration: 7 * 24 * time.Hour, wantPredict: 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var predictHours uint8

			switch {
			case tt.duration <= time.Hour:
				predictHours = 1
			case tt.duration <= 4*time.Hour:
				predictHours = 2
			case tt.duration <= 12*time.Hour:
				predictHours = 4
			case tt.duration <= 24*time.Hour:
				predictHours = 6
			default:
				predictHours = 12
			}

			if predictHours != tt.wantPredict {
				t.Errorf("predictHours for %v = %d, want %d", tt.duration, predictHours, tt.wantPredict)
			}
		})
	}
}

func TestAdminAuthorization(t *testing.T) {
	tests := []struct {
		name       string
		adminIDs   map[int64]struct{}
		userID     int64
		wantAccess bool
	}{
		{
			name:       "admin user has access",
			adminIDs:   map[int64]struct{}{123: {}},
			userID:     123,
			wantAccess: true,
		},
		{
			name:       "non-admin user denied",
			adminIDs:   map[int64]struct{}{123: {}},
			userID:     456,
			wantAccess: false,
		},
		{
			name:       "empty admin list denies all",
			adminIDs:   map[int64]struct{}{},
			userID:     123,
			wantAccess: false,
		},
		{
			name:       "nil admin list denies all",
			adminIDs:   nil,
			userID:     123,
			wantAccess: false,
		},
		{
			name:       "multiple admins - first admin",
			adminIDs:   map[int64]struct{}{100: {}, 200: {}, 300: {}},
			userID:     100,
			wantAccess: true,
		},
		{
			name:       "multiple admins - last admin",
			adminIDs:   map[int64]struct{}{100: {}, 200: {}, 300: {}},
			userID:     300,
			wantAccess: true,
		},
		{
			name:       "zero user ID not in list",
			adminIDs:   map[int64]struct{}{123: {}},
			userID:     0,
			wantAccess: false,
		},
		{
			name:       "negative user ID check",
			adminIDs:   map[int64]struct{}{-1: {}},
			userID:     -1,
			wantAccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := tt.adminIDs[tt.userID]
			if ok != tt.wantAccess {
				t.Errorf("admin access for user %d = %v, want %v", tt.userID, ok, tt.wantAccess)
			}
		})
	}
}

func TestSendErrorMessage(t *testing.T) {
	mock := &mockBot{}
	ctx := context.Background()
	chatID := int64(12345)

	tests := []struct {
		name       string
		err        error
		text       string
		sendMsgErr error
	}{
		{
			name: "with error",
			err:  errors.New("test error"),
			text: "error message",
		},
		{
			name: "without error (nil)",
			err:  nil,
			text: "simple message",
		},
		{
			name:       "send message fails",
			err:        errors.New("original error"),
			text:       "error message",
			sendMsgErr: errors.New("send failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.mu.Lock()
			mock.sentMessages = nil
			mock.sendMsgErr = tt.sendMsgErr
			mock.mu.Unlock()

			_, err := mock.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   tt.text,
			})

			msgs := mock.getSentMessages()
			if len(msgs) != 1 {
				t.Errorf("expected 1 sent message, got %d", len(msgs))
				return
			}

			if msgs[0].ChatID != chatID {
				t.Errorf("chatID = %d, want %d", msgs[0].ChatID, chatID)
			}
			if msgs[0].Text != tt.text {
				t.Errorf("text = %q, want %q", msgs[0].Text, tt.text)
			}

			if tt.sendMsgErr != nil && err == nil {
				t.Error("expected error from SendMessage")
			}
		})
	}
}

func TestBuildGraphWithInsufficientData(t *testing.T) {
	db := newTestDB(t)
	cfg := newTestConfig(t, nil)
	h := NewBotHandler(db, cfg, nil)
	ctx := context.Background()

	tests := []struct {
		name       string
		events     []databaser.Event
		wantErrMsg string
	}{
		{
			name:       "no events",
			events:     nil,
			wantErrMsg: "Слишком мало данных за указанный период для построения графика",
		},
		{
			name: "single event",
			events: []databaser.Event{
				{Timestamp: time.Now().UTC(), Load: 50},
			},
			wantErrMsg: "Слишком мало данных за указанный период для построения графика",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.events) > 0 {
				if err := db.SaveManyEvents(ctx, tt.events); err != nil {
					t.Fatalf("failed to save events: %v", err)
				}
			}

			if h.db == nil {
				t.Error("handler db is nil")
			}
			if h.cfg == nil {
				t.Error("handler cfg is nil")
			}
		})
	}
}

func TestDateTimeFormat(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}

	tests := []struct {
		name    string
		ts      time.Time
		wantFmt string
	}{
		{
			name:    "standard time",
			ts:      time.Date(2024, 3, 15, 14, 30, 0, 0, loc),
			wantFmt: "15.03.2024 14:30",
		},
		{
			name:    "midnight",
			ts:      time.Date(2024, 1, 1, 0, 0, 0, 0, loc),
			wantFmt: "01.01.2024 00:00",
		},
		{
			name:    "end of day",
			ts:      time.Date(2024, 12, 31, 23, 59, 0, 0, loc),
			wantFmt: "31.12.2024 23:59",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted := tt.ts.Format(dateTimeFormat)
			if formatted != tt.wantFmt {
				t.Errorf("format = %q, want %q", formatted, tt.wantFmt)
			}
		})
	}
}

func TestMockBotMethods(t *testing.T) {
	ctx := context.Background()

	t.Run("SendMessage success", func(t *testing.T) {
		mock := &mockBot{}
		msg, err := mock.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: int64(123),
			Text:   "test",
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if msg == nil {
			t.Error("expected message, got nil")
		}
		if msg.Chat.ID != 123 {
			t.Errorf("chat ID = %d, want 123", msg.Chat.ID)
		}
	})

	t.Run("SendMessage error", func(t *testing.T) {
		mock := &mockBot{sendMsgErr: errors.New("test error")}
		_, err := mock.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: int64(123),
			Text:   "test",
		})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("SendPhoto success", func(t *testing.T) {
		mock := &mockBot{}
		msg, err := mock.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID: int64(456),
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if msg == nil {
			t.Error("expected message, got nil")
		}
	})

	t.Run("SendPhoto error", func(t *testing.T) {
		mock := &mockBot{sendPhotoErr: errors.New("photo error")}
		_, err := mock.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID: int64(456),
		})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("AnswerCallbackQuery success", func(t *testing.T) {
		mock := &mockBot{}
		ok, err := mock.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: "cb123",
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !ok {
			t.Error("expected true, got false")
		}
	})

	t.Run("AnswerCallbackQuery error", func(t *testing.T) {
		mock := &mockBot{answerCBErr: errors.New("cb error")}
		ok, err := mock.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: "cb123",
		})
		if err == nil {
			t.Error("expected error, got nil")
		}
		if ok {
			t.Error("expected false, got true")
		}
	})
}

func TestMockBotConcurrentAccess(t *testing.T) {
	mock := &mockBot{}
	ctx := context.Background()
	const goroutines = 10

	var wg sync.WaitGroup
	wg.Add(goroutines * 3)

	for range goroutines {
		go func() {
			defer wg.Done()
			_, err := mock.SendMessage(ctx, &bot.SendMessageParams{ChatID: 1, Text: "test"})
			if err != nil {
				t.Errorf("SendMessage error: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			_, err := mock.SendPhoto(ctx, &bot.SendPhotoParams{ChatID: 1})
			if err != nil {
				t.Errorf("SendPhoto error: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			_, err := mock.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: "1"})
			if err != nil {
				t.Errorf("AnswerCallbackQuery error: %v", err)
			}
		}()
	}

	wg.Wait()

	msgs := mock.getSentMessages()
	if len(msgs) != goroutines {
		t.Errorf("sent messages = %d, want %d", len(msgs), goroutines)
	}

	photos := mock.getSentPhotos()
	if len(photos) != goroutines {
		t.Errorf("sent photos = %d, want %d", len(photos), goroutines)
	}

	cbs := mock.getAnsweredCBs()
	if len(cbs) != goroutines {
		t.Errorf("answered callbacks = %d, want %d", len(cbs), goroutines)
	}
}

func TestDurationParsing(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{name: "valid hours", input: "2h", want: 2 * time.Hour, wantErr: false},
		{name: "valid minutes", input: "30m", want: 30 * time.Minute, wantErr: false},
		{name: "valid complex", input: "1h30m", want: 90 * time.Minute, wantErr: false},
		{name: "valid days (as hours)", input: "24h", want: 24 * time.Hour, wantErr: false},
		{name: "invalid format", input: "abc", want: 0, wantErr: true},
		{name: "empty string", input: "", want: 0, wantErr: true},
		{name: "negative duration", input: "-1h", want: -time.Hour, wantErr: false},
		{name: "zero duration", input: "0s", want: 0, wantErr: false},
		{name: "seconds", input: "3600s", want: time.Hour, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := time.ParseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDuration(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestBotHandlerFields(t *testing.T) {
	db := newTestDB(t)
	cfg := newTestConfig(t, map[int64]struct{}{123: {}})

	h := NewBotHandler(db, cfg, nil)

	t.Run("db field", func(t *testing.T) {
		if h.db != db {
			t.Error("db field not set correctly")
		}
	})

	t.Run("cfg field", func(t *testing.T) {
		if h.cfg != cfg {
			t.Error("cfg field not set correctly")
		}
	})

	t.Run("pc field nil", func(t *testing.T) {
		if h.pc != nil {
			t.Error("pc field should be nil")
		}
	})

	t.Run("admin IDs accessible", func(t *testing.T) {
		if _, ok := h.cfg.Base.AdminIDs[123]; !ok {
			t.Error("admin ID 123 should be accessible")
		}
	})

	t.Run("timezone accessible", func(t *testing.T) {
		if h.cfg.Base.TimeLocation == nil {
			t.Error("TimeLocation should not be nil")
		}
		if h.cfg.Base.TimeLocation.String() != "Europe/Moscow" {
			t.Errorf("TimeLocation = %s, want Europe/Moscow", h.cfg.Base.TimeLocation.String())
		}
	})
}

func TestEventStorage(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	events := []databaser.Event{
		{Timestamp: now.Add(-2 * time.Hour), Load: 30},
		{Timestamp: now.Add(-1 * time.Hour), Load: 50},
		{Timestamp: now, Load: 70},
	}

	if err := db.SaveManyEvents(ctx, events); err != nil {
		t.Fatalf("SaveManyEvents() error = %v", err)
	}

	retrieved, err := db.GetEvents(ctx, 3*time.Hour)
	if err != nil {
		t.Fatalf("GetEvents() error = %v", err)
	}

	if len(retrieved) != len(events) {
		t.Errorf("GetEvents() returned %d events, want %d", len(retrieved), len(events))
	}

	for i := 1; i < len(retrieved); i++ {
		if retrieved[i].Timestamp.Before(retrieved[i-1].Timestamp) {
			t.Error("events are not ordered by timestamp")
		}
	}
}

func TestHandlerWithEventsInDB(t *testing.T) {
	db := newTestDB(t)
	cfg := newTestConfig(t, map[int64]struct{}{100: {}})
	h := NewBotHandler(db, cfg, nil)
	ctx := context.Background()
	now := time.Now().UTC()

	events := []databaser.Event{
		{Timestamp: now.Add(-3 * time.Hour), Load: 20},
		{Timestamp: now.Add(-2 * time.Hour), Load: 40},
		{Timestamp: now.Add(-1 * time.Hour), Load: 60},
		{Timestamp: now, Load: 80},
	}

	if err := db.SaveManyEvents(ctx, events); err != nil {
		t.Fatalf("SaveManyEvents() error = %v", err)
	}

	retrieved, err := h.db.GetEvents(ctx, 4*time.Hour)
	if err != nil {
		t.Fatalf("GetEvents() through handler error = %v", err)
	}

	if len(retrieved) != len(events) {
		t.Errorf("got %d events, want %d", len(retrieved), len(events))
	}
}

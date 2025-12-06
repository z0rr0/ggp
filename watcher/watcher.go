// Package watcher provides functionality to monitor Telegram for new messages.
package watcher

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/z0rr0/ggp/config"
	"github.com/z0rr0/ggp/databaser"
	"github.com/z0rr0/ggp/plotter"
	"github.com/z0rr0/ggp/predictor"
)

// BotAPI defines the methods needed from the Telegram bot.
type BotAPI interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
	SendPhoto(ctx context.Context, params *bot.SendPhotoParams) (*models.Message, error)
}

// Telegram bot command constants.
const (
	CmdStart   = "/start"
	CmdWeek    = "📆 Неделя"
	CmdDay     = "📅 День"
	CmdHalfDay = "🕒 Полдня"
)

const (
	dateTimeFormat = "02.01.2006 15:04"
)

// BotHandler handles Telegram bot interactions for displaying load graphs.
type BotHandler struct {
	db  *databaser.DB
	cfg *config.Config
	pc  *predictor.Controller
}

// NewBotHandler creates a new BotHandler with the given dependencies.
func NewBotHandler(db *databaser.DB, cfg *config.Config, pc *predictor.Controller) *BotHandler {
	return &BotHandler{db: db, cfg: cfg, pc: pc}
}

// Wrapper methods for bot.HandlerFunc compatibility

// WrapHandleStart wraps HandleStart for bot.HandlerFunc compatibility.
func (h *BotHandler) WrapHandleStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.HandleStart(ctx, b, update)
}

// WrapHandleWeek wraps HandleWeek for bot.HandlerFunc compatibility.
func (h *BotHandler) WrapHandleWeek(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.HandleWeek(ctx, b, update)
}

// WrapHandleDay wraps HandleDay for bot.HandlerFunc compatibility.
func (h *BotHandler) WrapHandleDay(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.HandleDay(ctx, b, update)
}

// WrapHandleHalfDay wraps HandleHalfDay for bot.HandlerFunc compatibility.
func (h *BotHandler) WrapHandleHalfDay(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.HandleHalfDay(ctx, b, update)
}

// WrapDefaultHandler wraps DefaultHandler for bot.HandlerFunc compatibility.
func (h *BotHandler) WrapDefaultHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.DefaultHandler(ctx, b, update)
}

// HandleStart handles the /start command and shows the main keyboard.
func (h *BotHandler) HandleStart(ctx context.Context, b BotAPI, update *models.Update) {
	start := time.Now()
	defer func() {
		slog.InfoContext(ctx, "handle start completed", "duration", time.Since(start))
	}()

	if update.Message == nil {
		slog.WarnContext(ctx, "HandleStart: update.Message is nil")
		return
	}

	kb := &models.ReplyKeyboardMarkup{
		Keyboard: [][]models.KeyboardButton{
			{
				{Text: CmdDay},
				{Text: CmdWeek},
			},
			{
				{Text: CmdHalfDay},
			},
		},
		ResizeKeyboard: true,
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		Text:        "Выберите период",
		ReplyMarkup: kb,
	})

	if err != nil {
		slog.ErrorContext(ctx, "HandleStart", "error", err)
	}
}

// HandleWeek handles week-period load graph requests.
func (h *BotHandler) HandleWeek(ctx context.Context, b BotAPI, update *models.Update) {
	start := time.Now()
	const (
		predictHours uint8 = 12
		duration           = 7 * 24 * time.Hour
	)
	h.handlePeriod(ctx, b, update, duration, predictHours)
	slog.InfoContext(ctx, "HandleWeek completed", "duration", time.Since(start))
}

// HandleDay handles day period load graph requests.
func (h *BotHandler) HandleDay(ctx context.Context, b BotAPI, update *models.Update) {
	start := time.Now()
	const (
		predictHours uint8 = 6
		duration           = 24 * time.Hour
	)
	h.handlePeriod(ctx, b, update, duration, predictHours)
	slog.InfoContext(ctx, "HandleDay completed", "duration", time.Since(start))
}

// HandleHalfDay handles half-day period load graph requests.
func (h *BotHandler) HandleHalfDay(ctx context.Context, b BotAPI, update *models.Update) {
	start := time.Now()
	const (
		predictHours uint8 = 4
		duration           = 12 * time.Hour
	)
	h.handlePeriod(ctx, b, update, duration, predictHours)
	slog.InfoContext(ctx, "HandleHalfDay completed", "duration", time.Since(start))
}

// DefaultHandler handles all other messages, allowing admin users to request custom duration graphs.
func (h *BotHandler) DefaultHandler(ctx context.Context, b BotAPI, update *models.Update) {
	start := time.Now()
	defer func() {
		slog.InfoContext(ctx, "default handler completed", "duration", time.Since(start))
	}()

	if update.Message == nil {
		slog.WarnContext(ctx, "DefaultHandler: update.Message is nil")
		return
	}

	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID

	if !h.isAuthorized(userID) {
		slog.WarnContext(ctx, "unauthorized user", "userID", userID)
		return
	}

	text := update.Message.Text
	duration, err := time.ParseDuration(text)
	if err != nil {
		sendErrorMessage(ctx, err, b, chatID, "не удалось распознать период")
		return
	}

	predictHours := calculatePredictHours(duration)
	h.buildGraph(ctx, b, chatID, duration, predictHours)
}

// handlePeriod processes requests for load graphs over a specified duration.
func (h *BotHandler) handlePeriod(ctx context.Context, b BotAPI, update *models.Update, duration time.Duration, predictHours uint8) {
	if update.Message == nil || update.Message.From == nil {
		slog.WarnContext(ctx, "handlePeriod: update.Message or From is nil")
		return
	}

	userID := update.Message.From.ID
	if !h.isAuthorized(userID) {
		slog.WarnContext(ctx, "unauthorized user", "userID", userID)
		return
	}

	chatID := update.Message.Chat.ID
	text := update.Message.Text

	slog.DebugContext(ctx, "handlePeriod", "chatID", chatID, "userID", userID, "text", text)
	h.buildGraph(ctx, b, chatID, duration, predictHours)
}

// isAuthorized checks if the user is authorized to use the bot.
func (h *BotHandler) isAuthorized(userID int64) bool {
	_, ok := h.cfg.Base.AdminIDs[userID]
	return ok
}

// calculatePredictHours determines the number of prediction hours based on the duration.
func calculatePredictHours(duration time.Duration) uint8 {
	switch {
	case duration <= time.Hour:
		return 1
	case duration <= 4*time.Hour:
		return 2
	case duration <= 12*time.Hour:
		return 4
	case duration <= 24*time.Hour:
		return 6
	default:
		return 12
	}
}

func sendErrorMessage(ctx context.Context, err error, b BotAPI, chatID int64, text string) {
	if err != nil {
		slog.ErrorContext(ctx, "error occurred", "error", err, "message", text)
	}

	_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})

	if sendErr != nil {
		slog.ErrorContext(ctx, "failed to send error message", "error", sendErr, "message", text)
	}
}

func (h *BotHandler) buildGraph(ctx context.Context, b BotAPI, chatID int64, duration time.Duration, ph uint8) {
	events, err := h.db.GetEvents(ctx, duration)
	if err != nil {
		sendErrorMessage(ctx, err, b, chatID, "Не удалось получить данные за указанный период")
		return
	}

	n := len(events)
	if n < 2 {
		sendErrorMessage(ctx, nil, b, chatID, "Слишком мало данных за указанный период для построения графика")
		return
	}

	var prediction []databaser.Event
	if h.pc != nil {
		prediction = h.pc.PredictLoad(ph)
	}

	imageData, err := plotter.Graph(events, prediction, h.cfg.Base.TimeLocation)
	if err != nil {
		sendErrorMessage(ctx, err, b, chatID, "Не удалось построить график")
		return
	}

	slog.DebugContext(ctx, "graph", "image", len(imageData))
	caption := fmt.Sprintf(
		"%s - %s",
		events[0].Timestamp.In(h.cfg.Base.TimeLocation).Format(dateTimeFormat),
		events[n-1].Timestamp.In(h.cfg.Base.TimeLocation).Format(dateTimeFormat),
	)

	_, err = b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID: chatID,
		Photo: &models.InputFileUpload{
			Filename: "load.png",
			Data:     bytes.NewReader(imageData),
		},
		Caption: caption,
	})

	if err != nil {
		sendErrorMessage(ctx, err, b, chatID, "Не удалось отправить график")
		return
	}
}

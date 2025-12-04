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

const (
	StartCommand   = "/start"
	CallbackPrefix = "/period"
	MenuCommand    = "📋 Меню"

	callbackHalfDay = CallbackPrefix + "HalfDay"
	callbackDay     = CallbackPrefix + "Day"
	callbackWeek    = CallbackPrefix + "Week"

	dateTimeFormat = "02.01.2006 15:04"
)

type BotHandler struct {
	db  *databaser.DB
	cfg *config.Config
	pc  *predictor.Controller
}

func NewBotHandler(db *databaser.DB, cfg *config.Config, pc *predictor.Controller) *BotHandler {
	return &BotHandler{db: db, cfg: cfg, pc: pc}
}

func (h *BotHandler) HandleStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	start := time.Now()
	defer func() {
		slog.InfoContext(ctx, "handle start completed", "duration", time.Since(start))
	}()

	kb := &models.ReplyKeyboardMarkup{
		Keyboard: [][]models.KeyboardButton{
			{{Text: MenuCommand}},
		},
		ResizeKeyboard: true,
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		Text:        "Нажмите «Меню» для выбора периода",
		ReplyMarkup: kb,
	})

	if err != nil {
		slog.Error("HandleStart", "error", err)
	}
}

func (h *BotHandler) HandleMenu(ctx context.Context, b *bot.Bot, update *models.Update) {
	start := time.Now()
	defer func() {
		slog.InfoContext(ctx, "handle menu completed", "duration", time.Since(start))
	}()
	kb := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "📅 День", CallbackData: callbackDay},
				{Text: "📆 Неделя", CallbackData: callbackWeek},
			},
			{
				{Text: "🕒 Полдня", CallbackData: callbackHalfDay},
			},
		},
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		Text:        "Нужно выбрать период для отображения данных",
		ReplyMarkup: kb,
	})

	if err != nil {
		slog.Error("HandleMenu", "error", err)
	}
}

func (h *BotHandler) HandleCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	start := time.Now()
	defer func() {
		slog.InfoContext(ctx, "handle callback completed", "duration", time.Since(start))
	}()

	_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	if err != nil {
		slog.Error("answer callback query", "error", err)
	}

	chatID := update.CallbackQuery.Message.Message.Chat.ID
	userID := update.CallbackQuery.Message.Message.From.ID

	period := update.CallbackQuery.Data
	slog.DebugContext(ctx, "callback", "chatID", chatID, "userID", userID, "period", period)

	var (
		duration     time.Duration
		predictHours uint8 = 2
	)

	switch period {
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
		return
	}

	h.buildGraph(ctx, b, chatID, duration, predictHours)
}

func (h *BotHandler) DefaultHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	start := time.Now()
	defer func() {
		slog.InfoContext(ctx, "default handler completed", "duration", time.Since(start))
	}()

	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID

	if _, ok := h.cfg.Base.AdminIDs[userID]; !ok {
		slog.WarnContext(ctx, "unauthorized user", "userID", userID)
		return
	}

	text := update.Message.Text
	duration, err := time.ParseDuration(text)
	if err != nil {
		sendErrorMessage(ctx, err, b, chatID, "не удалось распознать период")
		return
	}

	var predictHours uint8

	switch {
	case duration <= time.Hour:
		predictHours = 1
	case duration <= 4*time.Hour:
		predictHours = 2
	case duration <= 12*time.Hour:
		predictHours = 4
	case duration <= 24*time.Hour:
		predictHours = 6
	default:
		predictHours = 12
	}

	h.buildGraph(ctx, b, chatID, duration, predictHours)
}

func sendErrorMessage(ctx context.Context, err error, b *bot.Bot, chatID int64, text string) {
	slog.ErrorContext(ctx, "sending error message", "error", err)

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})

	if err != nil {
		slog.ErrorContext(ctx, "failed to send message", "error", err)
	}
}

func (h *BotHandler) buildGraph(ctx context.Context, b *bot.Bot, chatID int64, duration time.Duration, ph uint8) {
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

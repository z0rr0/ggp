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
)

const (
	StartCommand   = "/start"
	CallbackPrefix = "/period"

	callbackDay  = CallbackPrefix + "Day"
	callbackWeek = CallbackPrefix + "Week"

	dateTimeFormat = "02.01.2006 15:04"
)

type BotHandler struct {
	db  *databaser.DB
	cfg *config.Config
}

func NewBotHandler(db *databaser.DB, cfg *config.Config) *BotHandler {
	return &BotHandler{db: db, cfg: cfg}
}

func (h *BotHandler) HandleStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	kb := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "📅 День", CallbackData: callbackDay},
			},
			{
				{Text: "📆 Неделя", CallbackData: callbackWeek},
			},
		},
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		Text:        "Нужно выбрать период для отображения данных",
		ReplyMarkup: kb,
	})

	if err != nil {
		slog.Error("send message", "error", err)
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

	var duration time.Duration

	switch period {
	case callbackDay:
		duration = 24 * time.Hour
	case callbackWeek:
		duration = 7 * 24 * time.Hour
	default:
		return
	}

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

	imageData, err := plotter.Graph(events, nil, h.cfg.Base.TimeLocation)
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

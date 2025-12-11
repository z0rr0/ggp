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
	"github.com/jmoiron/sqlx"

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
	CmdStart   = "start"
	CmdStop    = "stop"
	CmdID      = "id"
	CmdWeek    = "week"
	CmdDay     = "day"
	CmdHalfDay = "halfday"
)

const (
	dateTimeFormat = "02.01.2006 15:04"
)

var (
	// Commands defines the list of Telegram bot commands.
	Commands = []models.BotCommand{ //nolint:gochecknoglobals
		{
			Command:     CmdStart,
			Description: "Начать работу с ботом 🤖",
		},
		{
			Command:     CmdStop,
			Description: "Остановить работу с ботом 🛑",
		},
		{
			Command:     CmdHalfDay,
			Description: "Показать график за полдня 🕒",
		},
		{
			Command:     CmdDay,
			Description: "Показать график за день 📅",
		},
		{
			Command:     CmdWeek,
			Description: "Показать график за неделю 📆",
		},
		{
			Command:     CmdID,
			Description: "Показать ваш Telegram ID 🪪",
		},
	}
)

// BotHandler handles Telegram bot interactions for displaying load graphs.
type BotHandler struct {
	db       *databaser.DB
	cfg      *config.Config
	pc       *predictor.Controller
	adminIDs map[int64]struct{}
}

// NewBotHandler creates a new BotHandler with the given dependencies.
func NewBotHandler(db *databaser.DB, cfg *config.Config, pc *predictor.Controller) *BotHandler {
	return &BotHandler{db: db, cfg: cfg, pc: pc, adminIDs: cfg.Base.AdminIDs}
}

// Wrapper methods for bot.HandlerFunc compatibility

// WrapHandleStart wraps HandleStart for bot.HandlerFunc compatibility.
func (h *BotHandler) WrapHandleStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.HandleStart(ctx, b, update)
}

// WrapHandleStop wraps HandleStop for bot.HandlerFunc compatibility.
func (h *BotHandler) WrapHandleStop(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.HandleStop(ctx, b, update)
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

// WrapHandleID wraps HandleID for bot.HandlerFunc compatibility.
func (h *BotHandler) WrapHandleID(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.HandleID(ctx, b, update)
}

// WrapDefaultHandler wraps DefaultHandler for bot.HandlerFunc compatibility.
func (h *BotHandler) WrapDefaultHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.DefaultHandler(ctx, b, update)
}

// HandleStart handles the /start command and shows the main keyboard.
func (h *BotHandler) HandleStart(ctx context.Context, b BotAPI, update *models.Update) {
	if _, ok := h.adminIDs[update.Message.From.ID]; ok {
		sendErrorMessage(ctx, nil, b, update.Message.Chat.ID, "Вы являетесь администратором бота.")
		return
	}

	userFrom := update.Message.From
	var user *databaser.User

	tnxErr := databaser.InTransaction(ctx, h.db, func(tx *sqlx.Tx) error {
		dbUser, err := databaser.GetOrCreateUser(ctx, tx, userFrom.ID, userFrom.Username, userFrom.FirstName, userFrom.LastName)
		if err != nil {
			return err
		}

		user = dbUser
		return nil
	})

	if tnxErr != nil {
		slog.ErrorContext(ctx, "HandleStart get or create user", "error", tnxErr)
		sendErrorMessage(ctx, tnxErr, b, update.Message.Chat.ID, "Не удалось обработать ваш запрос")
		return
	}

	var text string

	switch {
	case user.IsPending():
		text = "Ваш запрос принят, дождитесь подтверждения."
	case user.IsApproved():
		text = "Бот уже активен. Используйте команды для получения графиков."
	default:
		text = "Ваш запрос отклонён."
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   text,
	})

	if err != nil {
		slog.ErrorContext(ctx, "HandleStart", "error", err)
	}

	// notify admins about new users
	adminText := fmt.Sprintf(
		"Пользователь запросил доступ (статус=%d):\nID: %d\n@%s %s %s",
		user.Status,
		user.ID,
		user.Username,
		user.FirstName,
		user.LastName,
	)
	for adminID := range h.adminIDs {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: adminID,
			Text:   adminText,
		})

		if err != nil {
			slog.ErrorContext(ctx, "HandleStart notify admin", "adminID", adminID, "userID", user.ID, "error", err)
		}
	}
}

// HandleStop handles the /stop command and removes the main keyboard.
func (h *BotHandler) HandleStop(ctx context.Context, b BotAPI, update *models.Update) {
	err := h.db.DeleteUser(ctx, update.Message.From.ID)
	if err != nil {
		sendErrorMessage(ctx, err, b, update.Message.Chat.ID, "Не удалось обработать ваш запрос")
		return
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "Бот остановлен. Чтобы начать снова, используйте команду /start.",
	})

	if err != nil {
		slog.ErrorContext(ctx, "HandleStop", "error", err)
	}
}

// HandleWeek handles week-period load graph requests.
func (h *BotHandler) HandleWeek(ctx context.Context, b BotAPI, update *models.Update) {
	const (
		predictHours uint8 = 12
		duration           = 7 * 24 * time.Hour
	)
	h.handlePeriod(ctx, b, update, duration, predictHours)
}

// HandleDay handles day period load graph requests.
func (h *BotHandler) HandleDay(ctx context.Context, b BotAPI, update *models.Update) {
	const (
		predictHours uint8 = 6
		duration           = 24 * time.Hour
	)
	h.handlePeriod(ctx, b, update, duration, predictHours)
}

// HandleHalfDay handles half-day period load graph requests.
func (h *BotHandler) HandleHalfDay(ctx context.Context, b BotAPI, update *models.Update) {
	const (
		predictHours uint8 = 4
		duration           = 12 * time.Hour
	)
	h.handlePeriod(ctx, b, update, duration, predictHours)
}

// HandleID handles the /id command and returns the user's Telegram ID.
func (h *BotHandler) HandleID(ctx context.Context, b BotAPI, update *models.Update) {
	userID := update.Message.From.ID
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   fmt.Sprintf("ID: %d", userID),
	})
	if err != nil {
		slog.ErrorContext(ctx, "HandleID", "error", err)
	}
}

// DefaultHandler handles all other messages, allowing admin users to request custom duration graphs.
func (h *BotHandler) DefaultHandler(ctx context.Context, b BotAPI, update *models.Update) {
	if emptyUpdate(update) {
		slog.WarnContext(ctx, "default handler: update is nil")
		return
	}

	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID

	if !h.isAdmin(userID) {
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
	userID := update.Message.From.ID
	chatID := update.Message.Chat.ID
	text := update.Message.Text

	slog.DebugContext(ctx, "handlePeriod", "chatID", chatID, "userID", userID, "text", text)
	h.buildGraph(ctx, b, chatID, duration, predictHours)
}

// isAdmin checks if the user is authorized to use the bot.
func (h *BotHandler) isAdmin(userID int64) bool {
	_, ok := h.adminIDs[userID]
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

// buildGraph constructs and sends the load graph to the user.
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

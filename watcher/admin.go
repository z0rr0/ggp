package watcher

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Admin bot command constants.
const (
	CmdUsers   = "users"
	CmdApprove = "approve"
	CmdReject  = "reject"
)

// WrapHandleUsers wraps HandleUsers to match bot.HandlerFunc signature.
func (h *BotHandler) WrapHandleUsers(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.HandleUsers(ctx, b, update)
}

// WrapHandleApprove wraps HandleApprove to match bot.HandlerFunc signature.
func (h *BotHandler) WrapHandleApprove(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.HandleApprove(ctx, b, update)
}

// WrapHandleReject wraps HandleReject to match bot.HandlerFunc signature.
func (h *BotHandler) WrapHandleReject(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.HandleReject(ctx, b, update)
}

// HandleUsers returns users information.
func (h *BotHandler) HandleUsers(ctx context.Context, b BotAPI, update *models.Update) {
	const (
		approvedSymbol = "✅"
		pendingSymbol  = "⏳"
		rejectedSymbol = "❌"
	)

	users, err := h.db.GetUsers(ctx)
	if err != nil {
		sendErrorMessage(ctx, err, b, update.Message.Chat.ID, "Не удалось получить список пользователей.")
		return
	}

	var (
		sb     strings.Builder
		status string
	)
	sb.WriteString("Пользователи:\n")

	for _, user := range users {
		switch {
		case user.IsApproved():
			status = approvedSymbol
		case user.IsPending():
			status = pendingSymbol
		default:
			status = rejectedSymbol
		}
		sb.WriteString(status)
		sb.WriteString(" ID: ")
		sb.WriteString(strconv.FormatInt(user.ID, 10))
		sb.WriteString(" @")
		sb.WriteString(user.Username)
		sb.WriteString(" ")
		sb.WriteString(user.FirstName)
		sb.WriteString(" ")
		sb.WriteString(user.LastName)
		sb.WriteString("\n")
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   sb.String(),
	})

	if err != nil {
		sendErrorMessage(ctx, err, b, update.Message.Chat.ID, "Не удалось отправить список пользователей.")
		return
	}
}

// HandleApprove approves a user by its ID.
func (h *BotHandler) HandleApprove(ctx context.Context, b BotAPI, update *models.Update) { //nolint:dupl
	args := strings.Fields(update.Message.Text)
	if len(args) < 2 {
		sendErrorMessage(ctx, nil, b, update.Message.Chat.ID, "Используйте: /approve <user_id>")
		return
	}

	userID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		sendErrorMessage(ctx, err, b, update.Message.Chat.ID, "Неверный формат user_id.")
		return
	}

	err = h.db.ApproveUser(ctx, userID)
	if err != nil {
		sendErrorMessage(ctx, err, b, update.Message.Chat.ID, "Не удалось одобрить пользователя.")
		return
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "Пользователь одобрен.",
	})

	if err != nil {
		sendErrorMessage(ctx, err, b, update.Message.Chat.ID, "Не удалось отправить подтверждение одобрения.")
		return
	}

	// notify users about approval
	slog.InfoContext(ctx, "approved user", "user_id", userID)
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: userID,
		Text:   "Ваш запрос одобрен администратором. Бот активен.",
	})
	if err != nil {
		slog.ErrorContext(ctx, "notify approved user", "user_id", userID, "error", err)
	}
}

// HandleReject rejects a user by its ID.
func (h *BotHandler) HandleReject(ctx context.Context, b BotAPI, update *models.Update) { //nolint:dupl
	args := strings.Fields(update.Message.Text)
	if len(args) < 2 {
		sendErrorMessage(ctx, nil, b, update.Message.Chat.ID, "Используйте: /reject <user_id>")
		return
	}

	userID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		sendErrorMessage(ctx, err, b, update.Message.Chat.ID, "Неверный формат user_id.")
		return
	}

	err = h.db.RejectUser(ctx, userID)
	if err != nil {
		sendErrorMessage(ctx, err, b, update.Message.Chat.ID, "Не удалось отклонить запрос пользователя.")
		return
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "Запрос отклонён.",
	})

	if err != nil {
		sendErrorMessage(ctx, err, b, update.Message.Chat.ID, "Не удалось отправить подтверждение отклонения.")
		return
	}

	// notify user about rejection
	slog.InfoContext(ctx, "rejected user", "user_id", userID)
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: userID,
		Text:   "Ваш запрос отклонён администратором.",
	})
	if err != nil {
		slog.ErrorContext(ctx, "notify rejected user", "user_id", userID, "error", err)
	}
}

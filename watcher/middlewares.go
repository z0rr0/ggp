package watcher

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/z0rr0/ggp/databaser"
)

const (
	// requestIDLen is a length of generated request ID in bytes.
	requestIDLen = 16
)

// BotLoggingMiddleware is a middleware that logs the start and stop of each request.
func BotLoggingMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		start := time.Now()
		requestID := generateRequestID()
		text := "undefined"

		defer func() {
			slog.InfoContext(ctx, "request stop", "id", requestID, "text", text, "duration", time.Since(start))
		}()

		if emptyUpdate(update) {
			slog.WarnContext(ctx, "update is nil")
			return
		}

		text = update.Message.Text
		slog.InfoContext(ctx, "request start", "id", requestID, "text", text)
		next(ctx, b, update)
	}
}

// BotAdminOnlyMiddleware is a middleware that allows only admin users to proceed.
func BotAdminOnlyMiddleware(adminUserIDs map[int64]struct{}) func(next bot.HandlerFunc) bot.HandlerFunc {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			if emptyUpdate(update) {
				slog.WarnContext(ctx, "admin middleware: update is nil")
				return
			}

			userID := update.Message.From.ID
			if _, ok := adminUserIDs[userID]; !ok {
				slog.InfoContext(ctx, "unauthorized admin user", "user_id", userID, "username", update.Message.From.Username)
				sendErrorMessage(ctx, nil, b, update.Message.Chat.ID, "Эта команда доступна только администраторам.")
				return
			}

			slog.DebugContext(ctx, "authorized admin", "user", userID)
			next(ctx, b, update)
		}
	}
}

// BotAuthMiddleware is a middleware that checks if the user is authorized.
// It's not a clean middleware, but a wrapper to prepare it with admin and DB users.
func BotAuthMiddleware(adminUserIDs map[int64]struct{}, db *databaser.DB) func(next bot.HandlerFunc) bot.HandlerFunc {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			if emptyUpdate(update) {
				slog.WarnContext(ctx, "auth middleware: update is nil")
				return
			}

			userID := update.Message.From.ID
			if _, ok := adminUserIDs[userID]; ok {
				next(ctx, b, update)
				return
			}

			// check if user exists and is approved
			user, err := db.GetUser(ctx, userID)
			if err != nil {
				slog.InfoContext(ctx, "user not found or error", "user_id", userID, "error", err)
				sendErrorMessage(ctx, nil, b, update.Message.Chat.ID, "Команда доступна только после запуска бота и подтверждения администраторами.")
				return
			}

			if !user.IsApproved() {
				slog.InfoContext(ctx, "user not approved", "user_id", userID, "username", update.Message.From.Username)
				sendErrorMessage(ctx, nil, b, update.Message.Chat.ID, "Команда доступна только после запуска бота и подтверждения администраторами.")
				return
			}

			slog.DebugContext(ctx, "authorized", "user", userID)
			next(ctx, b, update)
		}
	}
}

// generateRequestID generates a new request ID.
func generateRequestID() string {
	bytes := make([]byte, requestIDLen)
	_, err := io.ReadFull(rand.Reader, bytes)

	if err != nil {
		slog.Warn("failed to generate request ID", "error", err)
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}

	return hex.EncodeToString(bytes)
}

// emptyUpdate checks if the update is empty or invalid.
func emptyUpdate(update *models.Update) bool {
	return update.Message == nil || update.Message.From == nil
}

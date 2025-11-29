// Package main implements the GGP Telegram bot application.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/z0rr0/ggp/config"
	"github.com/z0rr0/ggp/databaser"
)

func main() {
	configPath := flag.String("config", "config.toml", "path to configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	db, err := databaser.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := db.Init(ctx); err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}

	opts := []bot.Option{
		bot.WithDefaultHandler(echoHandler(db)),
	}

	b, err := bot.New(cfg.Telegram.Token, opts...)
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}

	log.Println("bot started")
	b.Start(ctx)
}

// echoHandler returns a handler that echoes messages and saves them to the database.
func echoHandler(db *databaser.DB) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.Message == nil {
			return
		}

		msg := update.Message
		chatID := msg.Chat.ID
		userID := msg.From.ID
		text := msg.Text

		// Save message to database
		if err := db.SaveMessage(ctx, chatID, userID, text); err != nil {
			log.Printf("failed to save message: %v", err)
		}

		// Echo the message back
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   text,
		})
		if err != nil {
			log.Printf("failed to send message: %v", err)
		}
	}
}

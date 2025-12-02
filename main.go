// Package main implements the GGP Telegram bot application.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/z0rr0/ggp/config"
	"github.com/z0rr0/ggp/databaser"
	"github.com/z0rr0/ggp/fetcher"
	"github.com/z0rr0/ggp/importer"
	"github.com/z0rr0/ggp/plotter"
)

var (
	// Version is a git version
	Version = "v0.0.0"
	// Revision is a revision number
	Revision = "git:0000000"
	// BuildDate is a build date
	BuildDate = "1970-01-01T00:00:00"
	// GoVersion is a runtime Go language version
	GoVersion = runtime.Version() // "go1.00.0"
)

func main() {
	const name = "GGP"
	var (
		configPath = "config.toml"
		importPath string
	)

	defer func() {
		if r := recover(); r != nil {
			slog.Error("abnormal termination", "version", Version, "error", r)
			_, writeErr := fmt.Fprintf(os.Stderr, "abnormal termination: %v\n", string(debug.Stack()))
			if writeErr != nil {
				slog.Error("failed to write stack trace", "error", writeErr)
			}
		}
	}()

	flag.StringVar(&configPath, "config", configPath, "path to configuration file")
	flag.StringVar(&importPath, "import", importPath, "path to import data from CSV file")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// init slog logger
	initLogger(cfg.Base.Debug, os.Stdout)
	slog.Info(
		"Start",
		"name", name, "version", Version, "revision", Revision,
		"go", GoVersion, "build", BuildDate, "debug", cfg.Base.Debug,
	)

	dbCtx, dbCancel := context.WithTimeout(context.Background(), cfg.Database.Timeout)
	defer dbCancel()

	db, err := databaser.New(dbCtx, cfg.Database.Path)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer func() {
		if dbErr := db.Close(); dbErr != nil {
			slog.Error("failed to close database", "error", dbErr)
		}
	}()

	if importPath != "" {
		slog.Info("importing data", "path", importPath)
		err = importer.ImportCSV(db, importPath, cfg.Database.Timeout, cfg.Base.TimeLocation)
		if err != nil {
			slog.Error("failed to import data", "error", err)
		}
		return
	}

	// not importing, start bot
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err = db.Init(ctx); err != nil {
		slog.Error("failed to init database", "error", err)
		return
	}

	fetchDoneCh, err := runFetcher(ctx, cfg, db)
	if err != nil {
		slog.Error("failed to start fetcher", "error", err)
		return
	}

	err = runTelegramBot(ctx, cfg, db)
	if err != nil {
		slog.Error("telegram bot failed", "error", err)
		return
	}

	// wait for termination
	slog.Info("shutting down bot")
	<-ctx.Done()
	<-fetchDoneCh
	slog.Info("stopped")
}

func runTelegramBot(ctx context.Context, cfg *config.Config, db *databaser.DB) error {
	if !cfg.Telegram.Active {
		slog.Info("telegram bot is inactive")
		return nil
	}

	opts := []bot.Option{bot.WithDefaultHandler(telegramHandler(ctx, db, cfg.Base.TimeLocation, cfg.Base.AdminIDs))}
	b, err := bot.New(cfg.Telegram.Token, opts...)
	if err != nil {
		return fmt.Errorf("failed to create bot: %v", err)
	}

	slog.Info("bot started")
	b.Start(ctx)
	return nil
}

// telegramHandler returns a handler that echoes messages and saves them to the database.
func telegramHandler(ctx context.Context, db *databaser.DB, location *time.Location, admins map[int64]struct{}) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.Message == nil {
			return
		}

		msg := update.Message
		if _, ok := admins[msg.From.ID]; !ok {
			slog.WarnContext(ctx, "unauthorized user", "userID", msg.From.ID)
			return
		}

		chatID := msg.Chat.ID
		userID := msg.From.ID
		text := msg.Text
		slog.DebugContext(ctx, "telegramHandler", "chatID", chatID, "text", text, "userID", userID)

		duration, err := time.ParseDuration(text)
		if err != nil {
			slog.DebugContext(ctx, "duration parse", "chatID", chatID, "text", text, "error", err)
			duration = 24 * time.Hour
		}

		events, err := db.GetEvents(ctx, duration)
		if err != nil {
			slog.Error("failed to get events", "error", err)
			_, err = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "failed to get events",
			})
			if err != nil {
				slog.Error("failed to send message", "error", err)
			}
			return
		}
		n := len(events)
		if n < 2 {
			_, err = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "Too few data points in the period to plot a graph",
			})
			if err != nil {
				slog.Error("failed to send message", "error", err)
			}
			return
		}

		imageData, err := plotter.Graph(events, nil, location)
		if err != nil {
			slog.Error("failed to plot graph", "error", err)
			_, err = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "failed to plot graph",
			})
			if err != nil {
				slog.Error("failed to send message", "error", err)
			}
			return
		}

		slog.DebugContext(ctx, "telegramHandler", "imageSize", len(imageData))
		caption := fmt.Sprintf(
			"%s - %s",
			events[0].Timestamp.In(location).Format(time.DateTime),
			events[n-1].Timestamp.In(location).Format(time.DateTime),
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
			log.Printf("failed to send message: %v", err)
		}
	}
}

// initLogger initializes logger with debug mode and writer.
func initLogger(debug bool, w io.Writer) {
	var (
		level     = slog.LevelInfo
		addSource = false
	)
	if debug {
		addSource = true
		level = slog.LevelDebug
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{AddSource: addSource, Level: level})))
}

func runFetcher(ctx context.Context, cfg *config.Config, db *databaser.DB) (<-chan struct{}, error) {
	if !cfg.Fetcher.Active {
		slog.Info("fetcher is inactive")
		doneCh := make(chan struct{})
		close(doneCh)
		return doneCh, nil
	}

	fetchWorker := &fetcher.Fetcher{
		Db:           db,
		URL:          cfg.Fetcher.URL,
		Token:        cfg.Fetcher.AuthToken(),
		Timeout:      cfg.Fetcher.Timeout,
		QueryTimeout: cfg.Database.Timeout,
		Client:       &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}},
	}

	return fetchWorker.Run(ctx)
}

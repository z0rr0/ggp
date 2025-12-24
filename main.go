// Package main implements the GGP Telegram bot application.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"
	_ "time/tzdata"

	"github.com/go-telegram/bot"

	"github.com/z0rr0/ggp/config"
	"github.com/z0rr0/ggp/databaser"
	"github.com/z0rr0/ggp/fetcher"
	"github.com/z0rr0/ggp/holidayer"
	"github.com/z0rr0/ggp/importer"
	"github.com/z0rr0/ggp/predictor"
	"github.com/z0rr0/ggp/watcher"
)

var (
	// Version is a git version.
	Version = "v0.0.0" //nolint:gochecknoglobals
	// Revision is a revision number.
	Revision = "git:0000000" //nolint:gochecknoglobals
	// BuildDate is a build date.
	BuildDate = "1970-01-01T00:00:00" //nolint:gochecknoglobals
	// GoVersion is a runtime Go language version.
	GoVersion = runtime.Version() //nolint:gochecknoglobals
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
		slog.Error("failed to load config", "error", err)
		return
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

	db, err := databaser.New(dbCtx, cfg.Database.Path, cfg.Database.Threads)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		return
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

	fetchDoneCh, eventCh, err := runFetcher(ctx, cfg, db)
	if err != nil {
		slog.Error("failed to start fetcher", "error", err)
		return
	}

	holidayerDoneCh, err := runHolidayer(ctx, cfg, db)
	if err != nil {
		slog.Error("failed to start holidayer", "error", err)
		return
	}

	predictorCtr, predictorCh, err := runPredictor(ctx, cfg, db, eventCh)
	if err != nil {
		slog.Error("failed to start predictor", "error", err)
		return
	}

	err = runTelegramBot(ctx, cfg, db, predictorCtr)
	if err != nil {
		slog.Error("telegram bot failed", "error", err)
		return
	}

	// wait for termination
	slog.Info("shutting down bot")
	<-ctx.Done()
	<-predictorCh
	<-holidayerDoneCh
	<-fetchDoneCh
	slog.Info("stopped")
}

func runTelegramBot(ctx context.Context, cfg *config.Config, db *databaser.DB, pc *predictor.Controller) error {
	if !cfg.Telegram.Active {
		slog.Info("telegram bot is inactive")
		return nil
	}
	var (
		mwLog   bot.Middleware = watcher.BotLoggingMiddleware
		mwAuth  bot.Middleware = watcher.BotAuthMiddleware(cfg.Base.AdminIDs, db)
		mwAdmin bot.Middleware = watcher.BotAdminOnlyMiddleware(cfg.Base.AdminIDs)
	)

	botHandler := watcher.NewBotHandler(db, cfg, pc)
	b, err := bot.New(cfg.Telegram.Token, bot.WithDefaultHandler(mwLog(botHandler.WrapDefaultHandler)))
	if err != nil {
		return fmt.Errorf("failed to create bot: %w", err)
	}

	ok, err := b.SetMyCommands(ctx, &bot.SetMyCommandsParams{Commands: watcher.Commands})
	if err != nil {
		return fmt.Errorf("failed to set bot commands: %w", err)
	}
	if !ok {
		return errors.New("bot commands are not set")
	}

	b.RegisterHandler(bot.HandlerTypeMessageText, watcher.CmdStart, bot.MatchTypeCommand, botHandler.WrapHandleStart, mwLog)
	b.RegisterHandler(bot.HandlerTypeMessageText, watcher.CmdStop, bot.MatchTypeCommand, botHandler.WrapHandleStop, mwLog, mwAuth)
	b.RegisterHandler(bot.HandlerTypeMessageText, watcher.CmdID, bot.MatchTypeCommand, botHandler.WrapHandleID, mwLog, mwAuth)
	b.RegisterHandler(bot.HandlerTypeMessageText, watcher.CmdWeek, bot.MatchTypeCommand, botHandler.WrapHandleWeek, mwLog, mwAuth)
	b.RegisterHandler(bot.HandlerTypeMessageText, watcher.CmdDay, bot.MatchTypeCommand, botHandler.WrapHandleDay, mwLog, mwAuth)
	b.RegisterHandler(bot.HandlerTypeMessageText, watcher.CmdHalfDay, bot.MatchTypeCommand, botHandler.WrapHandleHalfDay, mwLog, mwAuth)

	// admin handlers
	b.RegisterHandler(bot.HandlerTypeMessageText, watcher.CmdUsers, bot.MatchTypeCommand, botHandler.WrapHandleUsers, mwLog, mwAdmin)
	b.RegisterHandler(bot.HandlerTypeMessageText, watcher.CmdApprove, bot.MatchTypeCommand, botHandler.WrapHandleApprove, mwLog, mwAdmin)
	b.RegisterHandler(bot.HandlerTypeMessageText, watcher.CmdReject, bot.MatchTypeCommand, botHandler.WrapHandleReject, mwLog, mwAdmin)

	slog.Info("bot is starting")
	b.Start(ctx)
	return nil
}

// initLogger initializes logger with debug mode and writer.
func initLogger(debug bool, w io.Writer) {
	var level = slog.LevelInfo

	if debug {
		level = slog.LevelDebug
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})))
}

func runFetcher(ctx context.Context, cfg *config.Config, db *databaser.DB) (<-chan struct{}, <-chan databaser.Event, error) {
	if !cfg.Fetcher.Active {
		slog.Info("fetcher is inactive")
		doneCh := make(chan struct{})
		close(doneCh)
		return doneCh, nil, nil
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

func runHolidayer(ctx context.Context, cfg *config.Config, db *databaser.DB) (<-chan struct{}, error) {
	if !cfg.Holidayer.Active {
		slog.Info("holidayer is inactive")
		doneCh := make(chan struct{})
		close(doneCh)
		return doneCh, nil
	}

	holidayerWorker := &holidayer.HolidayParams{
		Db:           db,
		Location:     cfg.Base.TimeLocation,
		URL:          cfg.Holidayer.URL,
		Timeout:      cfg.Holidayer.Timeout,
		QueryTimeout: cfg.Database.Timeout,
		Client:       &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}},
	}

	return holidayerWorker.Run(ctx)
}

func runPredictor(ctx context.Context, cfg *config.Config, db *databaser.DB, eventCh <-chan databaser.Event) (*predictor.Controller, <-chan struct{}, error) {
	if !cfg.Predictor.Active {
		slog.Info("predictor is inactive")
		doneCh := make(chan struct{})
		close(doneCh)
		return nil, doneCh, nil
	}

	controller, err := predictor.Run(ctx, db, eventCh, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start predictor controller: %w", err)
	}

	return controller, controller.Run(ctx), nil
}

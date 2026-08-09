package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"

	"github.com/google/uuid"

	"github.com/zachorosz/byom/images"
	"github.com/zachorosz/byom/metadata"
	"github.com/zachorosz/byom/scan"
	"github.com/zachorosz/byom/sqlite"
	"github.com/zachorosz/byom/storage"
)

var (
	debug = flag.Bool("debug", false, "Enable debug logging")
)

func main() {
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logger := setupLogger(*debug)

	db := mustSetupDB(ctx, "tmp/byom.db", logger)
	defer db.Close()

	loc := storage.Location{
		ID:        uuid.Must(uuid.NewV7()),
		URI:       "file:///mnt/music/Library",
		Available: true,
	}

	if err := db.QueryRowContext(ctx,
		`INSERT INTO locations (id, uri) VALUES (?, ?)
		 ON CONFLICT DO UPDATE SET uri=excluded.uri
		 RETURNING id`,
		loc.ID, loc.URI).Scan(&loc.ID); err != nil {
		logger.Error("seed location failed", slog.Any("error", err))
		os.Exit(1)
	}

	scanStore := sqlite.NewScanStore(db)
	parseQueueStore := sqlite.NewParseQueueStore(db)
	locations := sqlite.NewLocationStore(db)
	imageStore, err := images.NewStore(ctx, "tmp", sqlite.NewImageIndex(db))
	if err != nil {
		logger.Error("init image store failed", slog.Any("error", err))
		os.Exit(1)
	}

	importer := &metadata.Importer{Library: sqlite.NewLibraryStore(db)}

	pipeline := &metadata.Pipeline{
		Store:     parseQueueStore,
		Locations: locations,
		Images:    imageStore,
		Logger:    logger,
		OnResult:  importer.Import,
	}

	pipelineErr := make(chan error, 1)
	go func() { pipelineErr <- pipeline.Run(ctx) }()

	root, err := loc.Root()
	if err != nil {
		logger.Error("resolve location root failed", slog.Any("error", err))
		os.Exit(1)
	}

	scanner := &scan.Scanner{
		Store:   scanStore,
		Workers: 4,
		OnDirty: pipeline.Wake,
	}
	if err := scanner.Scan(ctx, os.DirFS(root), loc); err != nil {
		logger.Error("scan failed", slog.Any("error", err), slog.String("location_id", loc.ID.String()))
	} else {
		logger.Info("scan complete!", slog.String("location_id", loc.ID.String()))
	}

	<-ctx.Done()

	if err := <-pipelineErr; err != nil && !errors.Is(err, context.Canceled) {
		logger.Warn("parse pipeline failed", slog.Any("error", err))
	}
}

func mustSetupDB(ctx context.Context, dsn string, logger *slog.Logger) *sql.DB {
	db, err := sqlite.Open(dsn)
	if err != nil {
		logger.Error("open db failed", slog.Any("error", err), slog.String("dsn", dsn))
		os.Exit(1)
	}
	if err := sqlite.Migrate(ctx, db, logger); err != nil {
		logger.Error("db migrations failed", slog.Any("error", err))
		os.Exit(1)
	}
	return db
}

func setupLogger(debug bool) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	if debug {
		opts.Level = slog.LevelDebug
		opts.AddSource = true
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, opts))
	slog.SetDefault(logger)
	return logger
}

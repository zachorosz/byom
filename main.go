package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/google/uuid"

	"github.com/zachorosz/byom/images"
	"github.com/zachorosz/byom/metadata"
	"github.com/zachorosz/byom/rpc"
	"github.com/zachorosz/byom/scan"
	"github.com/zachorosz/byom/sqlite"
	"github.com/zachorosz/byom/storage"
)

var (
	debug = flag.Bool("debug", false, "Enable debug logging")
	addr  = flag.String("addr", "localhost:8080", "RPC server listen address")
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

	libraryStore := sqlite.NewLibraryStore(db)
	importer := &metadata.Importer{Library: libraryStore}

	pipeline := &metadata.Pipeline{
		Store:     parseQueueStore,
		Locations: locations,
		Images:    imageStore,
		Logger:    logger,
		OnResult:  importer.Import,
	}

	pipelineErr := make(chan error, 1)
	go func() { pipelineErr <- pipeline.Run(ctx) }()

	scanner := &scan.Scanner{
		Store:     scanStore,
		Locations: locations,
		Workers:   4,
		OnDirty:   pipeline.Wake,
		Logger:    logger,
	}
	if err := scanner.Recover(ctx); err != nil {
		logger.Error("recover scans failed", slog.Any("error", err))
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:    *addr,
		Handler: rpc.NewHandler(logger, rpc.NewLibraryServer(libraryStore)),
	}
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("rpc server listening", slog.String("addr", *addr))
		serveErr <- srv.ListenAndServe()
	}()

	if _, err := scanner.Start(ctx, loc.ID); err != nil {
		logger.Error("start scan failed", slog.Any("error", err), slog.String("location_id", loc.ID.String()))
	}

	select {
	case <-ctx.Done():
	case err := <-serveErr:
		logger.Error("rpc server failed", slog.Any("error", err))
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("rpc server shutdown failed", slog.Any("error", err))
	}
	if err := scanner.Shutdown(shutdownCtx); err != nil {
		logger.Warn("scanner shutdown failed", slog.Any("error", err))
	}

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

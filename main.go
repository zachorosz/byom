package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"os/signal"
	"runtime"

	"golang.org/x/sync/errgroup"

	"github.com/google/uuid"

	"github.com/zachorosz/byom/metadata"
	"github.com/zachorosz/byom/scan"
	"github.com/zachorosz/byom/sqlite"
	"github.com/zachorosz/byom/storage"
)

func newUUID() uuid.UUID {
	id, _ := uuid.NewV7()
	return id
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	db := mustSetupDB(ctx, "tmp/byom.db")
	defer db.Close()

	loc := storage.Location{
		ID:        newUUID(),
		URI:       "file:///mnt/music/Library",
		Available: true,
	}

	if err := db.QueryRowContext(ctx,
		`INSERT INTO locations (id, uri) VALUES (?, ?)
		 ON CONFLICT DO UPDATE SET uri=excluded.uri
		 RETURNING id`,
		loc.ID, loc.URI).Scan(&loc.ID); err != nil {
		slog.Error("seed location failed", slog.Any("error", err))
		os.Exit(1)
	}

	scanStore := sqlite.NewScanStore(db)
	parseQueueStore := sqlite.NewParseQueueStore(db)
	locations := sqlite.NewLocationStore(db)

	toParse := make(chan metadata.ClaimedDir, 100)
	parsed := make(chan metadata.ParseResult, 50)

	dispatcherGroup, dispatcherCtx := errgroup.WithContext(ctx)
	dispatcher := metadata.NewParseDispatcher(parseQueueStore, toParse)
	dispatcherGroup.Go(func() error { return dispatcher.Run(dispatcherCtx) })

	parserPool := metadata.StartParserPool(ctx, runtime.NumCPU(), parseQueueStore, locations, toParse, parsed)

	// TODO: do something with parsed data
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case res, ok := <-parsed:
				if !ok {
					return
				}
				slog.Info("handled parse result", slog.String("dir_id", res.DirID.String()))
			}
		}
	}()

	root, err := loc.Root()
	if err != nil {
		slog.Error("resolve location root failed", slog.Any("error", err))
		os.Exit(1)
	}

	scanner := &scan.Scanner{
		Store:   scanStore,
		Workers: 4,
		OnDirty: dispatcher.Wake,
	}
	if err := scanner.Scan(ctx, os.DirFS(root), loc); err != nil {
		slog.Error("scan failed", slog.Any("error", err), slog.String("location_id", loc.ID.String()))
	} else {
		slog.Info("scan complete!", slog.String("location_id", loc.ID.String()))
	}

	<-ctx.Done()

	if err := dispatcherGroup.Wait(); err != nil {
		slog.Warn("parse dispatcher failed", slog.Any("error", err))
	}
	if err := parserPool.Wait(); err != nil {
		slog.Warn("parser pool failed", slog.Any("error", err))
	}
}

func mustSetupDB(ctx context.Context, dsn string) *sql.DB {
	db, err := sqlite.Open(dsn)
	if err != nil {
		slog.Error("open db failed", slog.Any("error", err), slog.String("dsn", dsn))
		os.Exit(1)
	}
	if err := sqlite.Migrate(ctx, db, slog.Default()); err != nil {
		slog.Error("db migrations failed", slog.Any("error", err))
		os.Exit(1)
	}
	return db
}

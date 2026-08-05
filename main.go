package main

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/google/uuid"

	"github.com/zachorosz/byom/library"
	"github.com/zachorosz/byom/sqlite"
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

	st := library.Storage{
		ID:        newUUID(),
		URI:       "file:///mnt/music/Library",
		Available: true,
	}

	if err := db.QueryRowContext(ctx,
		`INSERT INTO storages (id, uri) VALUES (?, ?)
		 ON CONFLICT DO UPDATE SET uri=excluded.uri 
		 RETURNING id`,
		st.ID, st.URI).Scan(&st.ID); err != nil {
		slog.Error("seed storage failed", slog.Any("error", err))
		os.Exit(1)
	}

	libraryStore := sqlite.NewLibraryStore(db)

	toParse := make(chan ParsePayload, 100)
	parsed := make(chan ParseResult, 50)

	dispatcherGroup, dispatcherCtx := errgroup.WithContext(ctx)
	dispatcher := NewParseDispatcher(libraryStore, toParse)
	dispatcherGroup.Go(func() error { return dispatcher.Run(dispatcherCtx) })

	parserPool := StartParserPool(ctx, runtime.NumCPU(), libraryStore, toParse, parsed)

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

	startScan(ctx, libraryStore, st, dispatcher)

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
		slog.Error("FATAL: db migrations failed", slog.Any("error", err))
		os.Exit(1)
	}
	return db
}

func startScan(
	ctx context.Context,
	store *sqlite.LibraryStore,
	st library.Storage,
	dispatcher *ParseDispatcher,
) {
	gen, scanID, err := store.BeginScan(ctx, st.ID)
	if err != nil {
		slog.Error("begin scan failed", slog.Any("error", err))
		return
	}

	fsys := os.DirFS("/mnt/music/Library")
	dirs := make(chan walkResult, 32)

	syncPool := StartSyncPool(ctx, 4, store, st.ID, gen, dirs, dispatcher.Wake)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		defer close(dirs)
		merger := newDiscMerger(func(res walkResult) error {
			select {
			case <-gctx.Done():
				return gctx.Err()
			case dirs <- res:
			}
			return nil
		})
		return walkPostOrder(fsys, ".", merger.process)
	})

	walkErr := g.Wait()
	syncErr := syncPool.Wait()
	scanErr := errors.Join(walkErr, syncErr)

	// sweep dirs not found during this generation
	if _, err := store.Sweep(ctx, st.ID, gen); err != nil {
		slog.Warn("sweep failed", slog.Any("error", err))
	}

	if err := store.FinishScan(ctx, scanID, scanErr); err != nil {
		slog.Warn("finish scan failed", slog.Any("error", err))
	}

	slog.InfoContext(ctx, "scan complete!", slog.String("storage_id", st.ID.String()))
}

type walkResult struct {
	dir   string
	files []walkEntry
}

type walkEntry struct {
	fs.DirEntry
	kind library.FileKind
}

type prefixedEntry struct {
	fs.DirEntry
	prefix string
}

func (e prefixedEntry) Name() string { return path.Join(e.prefix, e.DirEntry.Name()) }

func walkPostOrder(fsys fs.FS, dir string, process func(walkResult) error) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		child := path.Join(dir, e.Name())
		if err := walkPostOrder(fsys, child, process); err != nil {
			return err
		}
	}

	res := walkResult{dir: dir}
	for _, e := range entries {
		if !e.Type().IsRegular() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		kind, ok := classifyKind(e)
		if !ok {
			continue
		}
		res.files = append(res.files, walkEntry{DirEntry: e, kind: kind})
	}

	return process(res)
}

var (
	audioExtensions = map[string]bool{
		".flac": true, ".mp3": true, ".ogg": true, ".m4a": true,
		".aiff": true, ".aif": true, ".wav": true, ".opus": true,
		".ape": true, ".wv": true,
	}
	imageExtensions = map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
		".gif": true, ".bmp": true, ".tiff": true, ".tif": true,
	}
)

func classifyKind(e fs.DirEntry) (library.FileKind, bool) {
	ext := strings.ToLower(filepath.Ext(e.Name()))
	switch {
	case audioExtensions[ext]:
		return library.FileAudio, true
	case imageExtensions[ext]:
		return library.FileImage, true
	default:
		return "", false
	}
}

var discPattern = regexp.MustCompile(`^(?i)(?:cd|disc)[\s._-]*\d+(?:[\s._-].*)?$`)

// isDisc reports whether dir's base name denotes a disc folder, such as "CD1"
// or "Disc 2 - Remixes". It inspects only the final path element, so
// "Album/CD1" is treated the same as "CD1".
func isDisc(dir string) bool {
	return discPattern.MatchString(path.Base(dir))
}

type discMerger struct {
	// pending holds walked dirs until their parent is seen.
	// fixme: leftover discs are never flushed.
	pending map[string]walkResult
	emit    func(walkResult) error
}

func newDiscMerger(emit func(walkResult) error) *discMerger {
	return &discMerger{
		pending: make(map[string]walkResult),
		emit:    emit,
	}
}

func (dm *discMerger) process(res walkResult) error {
	if isDisc(res.dir) { // ex: Album/CD1
		// buffer disc dir; will be merged when parent arrives.
		dm.pending[res.dir] = res
		return nil
	}
	// res is anchor (Album/)

	// collect any child discs
	var discs []walkResult
	for dir, disc := range dm.pending {
		parent := path.Dir(dir)
		if parent == res.dir {
			discs = append(discs, disc)
			delete(dm.pending, dir)
		}
	}

	if len(discs) == 0 {
		// flat album; pass it through
		return dm.emit(res)
	}

	// merge discs into this parent
	for _, disc := range discs {
		discName := filepath.Base(disc.dir)
		for _, f := range disc.files {
			res.files = append(res.files, walkEntry{
				DirEntry: prefixedEntry{DirEntry: f.DirEntry, prefix: discName},
				kind:     f.kind,
			})
		}
	}

	return dm.emit(res)
}

func StartSyncPool(
	ctx context.Context,
	workers int,
	store *sqlite.LibraryStore,
	storageID uuid.UUID,
	gen int64,
	in <-chan walkResult,
	onDirty func(),
) *errgroup.Group {
	g, gctx := errgroup.WithContext(ctx)
	for i := range workers {
		g.Go(func() error {
			return runSyncWorker(gctx, i, store, storageID, gen, in, onDirty)
		})
	}
	return g
}

func runSyncWorker(
	ctx context.Context,
	workerID int,
	store *sqlite.LibraryStore,
	storageID uuid.UUID,
	gen int64,
	in <-chan walkResult,
	onDirty func(),
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res, ok := <-in:
			if !ok {
				return nil
			}

			known := map[string]library.File{}

			dirID, err := store.DirID(ctx, storageID, res.dir)
			if err != nil && !errors.Is(err, library.ErrNotExists) {
				return err
			}
			if err == nil {
				known, err = store.KnownFiles(ctx, dirID)
				if err != nil {
					return err
				}
			}

			payload := library.SyncPayload{
				StorageID:  storageID,
				RelPath:    res.dir,
				Generation: gen,
			}
			payload.Changed, payload.Missing, payload.Dirty = computeChangeset(known, res)

			if _, err := store.SyncDir(ctx, payload); err != nil {
				return err
			}
			if payload.Dirty {
				onDirty()
			}
		}
	}
}

func computeChangeset(
	known map[string]library.File,
	res walkResult,
) (changed []library.File, missing []uuid.UUID, dirty bool) {
	for _, f := range res.files {
		name := f.Name()
		old, ok := known[name]
		if ok {
			delete(known, name)
		}
		info, err := f.Info()
		switch {
		case err != nil && ok: // any known file that fails to stat is missing for now.
			missing = append(missing, old.ID)
		case err == nil:
			cur := library.File{
				Name:    name,
				Kind:    f.kind,
				Size:    info.Size(),
				ModTime: info.ModTime(),
			}
			if ok {
				cur.ID = old.ID
			}
			if !old.Missing && old.Size == cur.Size && old.ModTime.Equal(cur.ModTime) {
				continue
			}
			changed = append(changed, cur)
		}
	}
	for _, leftover := range known {
		missing = append(missing, leftover.ID)
	}
	dirty = len(missing) > 0 || len(changed) > 0
	return
}

type ParseDispatcher struct {
	store       *sqlite.LibraryStore
	dirtyNotify chan struct{}
	toParse     chan<- ParsePayload
	logger      *slog.Logger
}

func NewParseDispatcher(store *sqlite.LibraryStore, toParse chan<- ParsePayload) *ParseDispatcher {
	return &ParseDispatcher{
		store:       store,
		dirtyNotify: make(chan struct{}, 1),
		toParse:     toParse,
		logger:      slog.Default(),
	}
}

// Wake is a coalescing trigger that notifies the dispatcher to wake up and
// start processing dirty directories.
func (d *ParseDispatcher) Wake() {
	select {
	case d.dirtyNotify <- struct{}{}:
	default:
		// dispatcher is already awake or a token is already queued.
		// we can safely drop this notification.
	}
}

func (d *ParseDispatcher) Run(ctx context.Context) error {
	// Startup sweep to process any dirty dirs leftover from previous runs.
	d.pump(ctx)

	// Wait for live triggers via Wake in an event loop.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-d.dirtyNotify:
			d.pump(ctx) // wake up and drain the DB until empty
		}
	}
}

// pump pulls from the DB and blocks if the parser queue is full.
// It returns when there are exactly 0 dirty directories left.
func (d *ParseDispatcher) pump(ctx context.Context) error {
	for {
		claimed, err := d.store.DirtyDirs(ctx, 50)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// TODO: maybe do something better here?
			d.logger.Error("fetch dirty directories failed", slog.Any("error", err))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1 * time.Second):
			}
			continue
		}
		if len(claimed) == 0 {
			return nil // DB is clean, exit pump and go back to sleep.
		}
		for _, c := range claimed {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case d.toParse <- ParsePayload{DirID: c.ID, LockedGeneration: c.LockedGeneration}:
				// if the parser queue is full, this gracefully blocks,
				// applying backpressure all the way up to this loop.
			}
		}
	}
}

type ParsePayload struct {
	DirID            uuid.UUID
	LockedGeneration int64
}

type ParseResult struct {
	DirID uuid.UUID
	// AlbumMetadata
	// AlbumArtists
	// Tracks
	// ParseErrors
}

func StartParserPool(
	ctx context.Context,
	workers int,
	store *sqlite.LibraryStore,
	toParse <-chan ParsePayload,
	out chan<- ParseResult,
) *errgroup.Group {
	g, gctx := errgroup.WithContext(ctx)
	for i := range workers {
		g.Go(func() error {
			return runParserWorker(gctx, i, store, toParse, out)
		})
	}
	return g
}

func runParserWorker(
	ctx context.Context,
	workerID int,
	store *sqlite.LibraryStore,
	in <-chan ParsePayload,
	out chan<- ParseResult,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case payload, ok := <-in:
			if !ok {
				return nil
			}

			files, err := store.DirFiles(ctx, payload.DirID)
			if err != nil {
				// A transient DB error here (SQLITE_BUSY, etc.) shouldn't
				// kill the whole pool, but it also can't be silently dropped,
				// since we hold a parse lock on this dir (locked_generation is
				// set).
				slog.Error("read dir files failed", slog.Any("error", err),
					slog.String("dir_id", payload.DirID.String()))

				// Release the lock but re-request a retry by leaving dirty
				// set.
				//
				// TODO: max retries and escalate to log.Error. if failing
				// because of something persistent (e.g corrupt row, dir deleted
				// mid-parse but not swept), we will spin on it every pump().
				if relErr := store.ReleaseAndRedirty(ctx, payload.DirID, payload.LockedGeneration); relErr != nil {
					slog.Warn("release+dirty failed", slog.Any("error", relErr),
						slog.String("dir_id", payload.DirID.String()))
				}
				continue
			}

			res := parseFiles(payload.DirID, files)

			if err := store.ReleaseDir(ctx, payload.DirID, payload.LockedGeneration); err != nil {
				// Lock already gone (crash-recovery reap, or duplicate
				// dispatch). Log but don't fail the parse itself, the
				// work already happened successfully.
				slog.Warn("release dir lock failed", slog.Any("error", err),
					slog.String("dir_id", payload.DirID.String()))
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case out <- res:
			}
		}
	}
}

func parseFiles(dirID uuid.UUID, _ []library.File) ParseResult {
	res := ParseResult{
		DirID: dirID,
	}
	return res
}

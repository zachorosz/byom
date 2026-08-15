package sqlite

import (
	"context"
	"database/sql"
	"runtime"
	"testing"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf(`Open(":memory:") failed: %v`, err)
	}
	t.Cleanup(func() { db.Close() })
	if err := Migrate(context.Background(), db, nil); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}
	return db
}

func TestOpen_ConfiguresConnectionPool(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf(`Open(":memory:") failed: %v`, err)
	}
	t.Cleanup(func() { db.Close() })

	want := max(4, runtime.NumCPU())
	if got := db.Stats().MaxOpenConnections; got != want {
		t.Errorf("MaxOpenConnections = %d, want %d", got, want)
	}
}

func TestOpen_ConfiguresPragmasOnEveryPoolConnection(t *testing.T) {
	ctx := context.Background()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf(`Open(":memory:") failed: %v`, err)
	}
	t.Cleanup(func() { db.Close() })

	// Hold connection #1 inside an open transaction so a concurrent
	// query below is forced onto a second, freshly-opened connection.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx(ctx, nil) failed: %v", err)
	}
	t.Cleanup(func() { tx.Rollback() })

	tests := []struct {
		pragma string
		want   int
	}{
		{"busy_timeout", 5000},
		{"foreign_keys", 1},
		{"synchronous", 1},
		{"temp_store", 2},
		{"cache_size", -64000},
	}
	for _, tc := range tests {
		var got int
		if err := db.QueryRowContext(ctx, "PRAGMA "+tc.pragma).Scan(&got); err != nil {
			t.Fatalf("PRAGMA %s failed: %v", tc.pragma, err)
		}
		if got != tc.want {
			t.Errorf("second connection: PRAGMA %s = %d, want %d", tc.pragma, got, tc.want)
		}
	}
}

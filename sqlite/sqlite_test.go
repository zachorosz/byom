package sqlite

import (
	"context"
	"database/sql"
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

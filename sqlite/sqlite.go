package sqlite

import (
	"database/sql"
	"fmt"
	"net/url"
	"runtime"
	"strings"

	_ "modernc.org/sqlite"
)

func Open(dsn string) (*sql.DB, error) {
	q := url.Values{}
	q.Set("_time_integer_format", "unix_nano")
	q.Set("_inttotime", "1")
	// PRAGMAs are per-connection: Setting them in the DSN makes
	// the driver apply them to every connection the pool ever opens.
	q.Set("_journal_mode", "wal")
	q.Set("_synchronous", "normal")
	q.Set("_busy_timeout", "5000")
	q.Set("_txlock", "immediate")
	q.Set("_foreign_keys", "1")
	q.Add("_pragma", "temp_store=MEMORY")
	q.Add("_pragma", "cache_size=-64000") // 64MB per connection

	if dsn == ":memory:" {
		dsn = "file::memory:?cache=shared&" + q.Encode()
	} else if strings.HasPrefix(dsn, "file:") {
		dsn = dsn + "&" + q.Encode()
	} else {
		dsn = fmt.Sprintf("file:%s?%s", dsn, q.Encode())
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	poolSize := max(4, runtime.NumCPU())
	db.SetMaxOpenConns(poolSize)
	db.SetMaxIdleConns(poolSize)

	return db, nil
}

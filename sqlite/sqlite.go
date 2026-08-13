package sqlite

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "modernc.org/sqlite"
)

func Open(dsn string) (*sql.DB, error) {
	q := url.Values{}
	q.Set("_time_integer_format", "unix_nano")
	q.Set("_inttotime", "1")

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
	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode = wal;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable wal: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	return db, nil
}

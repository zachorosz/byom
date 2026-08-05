package sqlite

import (
	"errors"

	"modernc.org/sqlite"
)

func isUniqueConstraintError(err error) bool {
	var e *sqlite.Error
	return errors.As(err, &e) && e.Code() == 2067
}

package apiframework

import (
	"fmt"
	"net/http"
	"testing"

	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/stretchr/testify/require"
)

// TestUnit_MapErrorToStatus_LibdbSentinels covers the libdbexec branch. Errors
// are WRAPPED, because that is how they arrive: the store wraps every driver
// error with context before a handler sees it, so a branch written with ==
// rather than errors.Is would pass a bare-sentinel test and fail in production.
func TestUnit_MapErrorToStatus_LibdbSentinels(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{libdb.ErrNotFound, http.StatusNotFound},
		{libdb.ErrUniqueViolation, http.StatusConflict},
		{libdb.ErrForeignKeyViolation, http.StatusConflict},
		{libdb.ErrNotNullViolation, http.StatusConflict},
		{libdb.ErrCheckViolation, http.StatusConflict},
		{libdb.ErrConstraintViolation, http.StatusConflict},
		{libdb.ErrMaxRowsReached, http.StatusTooManyRequests},
		{libdb.ErrDataTruncation, http.StatusBadRequest},
		{libdb.ErrNumericOutOfRange, http.StatusBadRequest},
		{libdb.ErrInvalidInputSyntax, http.StatusBadRequest},
		{libdb.ErrUndefinedColumn, http.StatusBadRequest},
		{libdb.ErrUndefinedTable, http.StatusBadRequest},
		{libdb.ErrDeadlockDetected, http.StatusConflict},
		{libdb.ErrSerializationFailure, http.StatusConflict},
		{libdb.ErrLockNotAvailable, http.StatusConflict},
		{libdb.ErrQueryCanceled, http.StatusConflict},
	}
	for _, tc := range cases {
		wrapped := fmt.Errorf("relay store: create account: %w", tc.err)
		require.Equal(t, tc.want, mapErrorToStatus(CreateOperation, wrapped), tc.err.Error())
	}
}

// TestUnit_MapErrorToStatus_AuthorizeStillWinsFirst guards the ordering
// mapErrorToStatus documents: AuthorizeOperation answers 403 ahead of the
// generic mapping, so a denied request is not reported as a missing row.
func TestUnit_MapErrorToStatus_AuthorizeStillWinsFirst(t *testing.T) {
	require.Equal(t, http.StatusForbidden,
		mapErrorToStatus(AuthorizeOperation, fmt.Errorf("denied: %w", libdb.ErrNotFound)))
}

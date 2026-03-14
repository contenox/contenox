package libdbexec

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/lib/pq"
)

// postgresDBManager implements the DBManager interface for PostgreSQL.
type postgresDBManager struct {
	dbInstance *sql.DB
}

// NewPostgresDBManager creates a new DBManager for PostgreSQL.
// It opens a connection group using the provided DSN, pings the database
// to verify connectivity, and optionally executes an initial schema setup query.
// Note: For production schema management, using dedicated migration tools is recommended
// over passing a simple schema string here.
func NewPostgresDBManager(ctx context.Context, dsn string, schema string) (DBManager, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		// Use translateError directly on the raw error
		return nil, fmt.Errorf("failed to open database: %w", translateError(err))
	}

	if err = db.PingContext(ctx); err != nil {
		_ = db.Close() // Attempt to close if ping fails
		return nil, fmt.Errorf("database connection failed: %w", translateError(err))
	}

	// Only execute schema if provided
	if schema != "" {
		if _, err = db.ExecContext(ctx, schema); err != nil {
			_ = db.Close() // Attempt to close if schema fails
			// Use translateError directly on the raw error
			return nil, fmt.Errorf("failed to initialize schema: %w", translateError(err))
		}
	}

	// log.Println("Database connection established and schema verified")
	return &postgresDBManager{dbInstance: db}, nil
}

// WithoutTransaction returns an executor that operates directly on the connection group.
func (sm *postgresDBManager) WithoutTransaction() Exec {
	return &txAwareDB{db: sm.dbInstance, errTranslate: translateError}
}

// WithTransaction starts a PostgreSQL transaction and returns the associated
// executor, commit function, and release function.
func (sm *postgresDBManager) WithTransaction(ctx context.Context, onRollback ...func()) (Exec, CommitTx, ReleaseTx, error) {
	// Use default transaction options. Could allow passing sql.TxOptions if needed.
	tx, err := sm.dbInstance.BeginTx(ctx, nil)
	if err != nil {
		// Use translateError on the raw error, wrap with ErrTxFailed context
		return nil, nil, func() error { return nil }, fmt.Errorf("%w: begin transaction failed: %w", ErrTxFailed, translateError(err))
	}

	// Executor bound to the transaction
	store := &txAwareDB{tx: tx, errTranslate: translateError}
	// finalized guards against double-execution of onRollback hooks when
	// releaseFn is deferred and commit also failed (both paths ran rollback logic).
	finalized := false
	fireRollback := func() {
		for _, f := range onRollback {
			if f != nil {
				f()
			}
		}
		onRollback = nil
	}
	commitFn := func(commitCtx context.Context) error {
		if ctxErr := commitCtx.Err(); ctxErr != nil {
			return fmt.Errorf("%w: context error before commit: %w", ErrTxFailed, ctxErr)
		}
		err := tx.Commit()
		if err != nil {
			return fmt.Errorf("%w: commit failed: %w", ErrTxFailed, translateError(err))
		}
		finalized = true
		return nil
	}

	releaseFn := func() error {
		rollbackErr := tx.Rollback()
		if !finalized {
			finalized = true
			fireRollback()
		}
		if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return fmt.Errorf("%w: rollback failed: %w", ErrTxFailed, translateError(rollbackErr))
		}
		return nil
	}

	return store, commitFn, releaseFn, nil
}

// Close shuts down the underlying database connection group.
func (sm *postgresDBManager) Close() error {
	if sm.dbInstance != nil {
		log.Println("Closing database connection group.")
		return sm.dbInstance.Close()
	}
	return nil
}

// txAwareDB implements the Exec interface, delegating to an underlying
// *sql.DB or *sql.Tx and translating errors via an injected translator.
// This allows each database driver (Postgres, SQLite, etc.) to wire
// in its own error mapping so sentinel errors like ErrUniqueViolation
// are always returned correctly regardless of driver.
type txAwareDB struct {
	db           *sql.DB
	tx           *sql.Tx
	errTranslate func(error) error // driver-specific error translator
}

// ExecContext delegates to the underlying DB or Tx and translates errors.
func (s *txAwareDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	var res sql.Result
	var err error
	if s.tx != nil {
		res, err = s.tx.ExecContext(ctx, query, args...)
	} else if s.db != nil {
		res, err = s.db.ExecContext(ctx, query, args...)
	} else {
		return nil, errors.New("libdb: Exec called on uninitialized txAwareDB")
	}
	return res, s.errTranslate(err)
}

// QueryContext delegates to the underlying DB or Tx and translates errors.
func (s *txAwareDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	var rows *sql.Rows
	var err error
	if s.tx != nil {
		rows, err = s.tx.QueryContext(ctx, query, args...)
	} else if s.db != nil {
		rows, err = s.db.QueryContext(ctx, query, args...)
	} else {
		return nil, errors.New("libdb: Query called on uninitialized txAwareDB")
	}
	if err != nil {
		return nil, s.errTranslate(err)
	}
	return rows, nil
}

// QueryRowContext delegates to the underlying DB or Tx and wraps the result.
func (s *txAwareDB) QueryRowContext(ctx context.Context, query string, args ...any) QueryRower {
	var r *sql.Row
	if s.tx != nil {
		r = s.tx.QueryRowContext(ctx, query, args...)
	} else if s.db != nil {
		r = s.db.QueryRowContext(ctx, query, args...)
	} else {
		return &row{err: errors.New("libdb: QueryRow called on uninitialized txAwareDB")}
	}
	return &row{inner: r, errTranslate: s.errTranslate}
}

// row implements QueryRower, wrapping *sql.Row to translate Scan errors.
type row struct {
	inner        *sql.Row
	err          error
	errTranslate func(error) error
}

// Scan calls the underlying Scan method and translates the error.
func (r *row) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if r.inner == nil {
		return errors.New("libdb: Scan called on nil row wrapper")
	}
	return r.errTranslate(r.inner.Scan(dest...))
}

// translateError translates common sql and pq errors into package-defined errors.
// It wraps unknown errors for context.
func translateError(err error) error {
	if err == nil {
		return nil
	}

	// Handle no rows error first - this is common after QueryRow().Scan().
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}

	// Handle context errors explicitly. Although checked elsewhere, they might
	// be returned directly by driver operations sometimes.
	if errors.Is(err, context.Canceled) {
		// Wrap context.Canceled with our specific error type if desired,
		// or just return a general query cancelled error.
		// Adding context.Canceled itself provides more detail via errors.Is/As.
		return fmt.Errorf("%w: %w", ErrQueryCanceled, context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrQueryCanceled, context.DeadlineExceeded)
	}

	// Check for PostgreSQL specific errors via pq.Error
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		// Use pqErr.Code which is the SQLSTATE code (e.g., "23505")
		// Using Code.Name() can be less stable if lib/pq changes names.
		switch pqErr.Code {
		case "23505":
			return fmt.Errorf("%w: %w", ErrUniqueViolation, err)
		case "23503":
			return fmt.Errorf("%w: %w", ErrForeignKeyViolation, err)
		case "23502":
			return fmt.Errorf("%w: %w", ErrNotNullViolation, err)
		case "23514":
			return fmt.Errorf("%w: %w", ErrCheckViolation, err)
		case "40P01":
			return fmt.Errorf("%w: %w", ErrDeadlockDetected, err)
		case "40001":
			return fmt.Errorf("%w: %w", ErrSerializationFailure, err)
		case "55P03":
			return fmt.Errorf("%w: %w", ErrLockNotAvailable, err)
		case "57014":
			return fmt.Errorf("%w: %w", ErrQueryCanceled, err)
		case "22001":
			return fmt.Errorf("%w: %w", ErrDataTruncation, err)
		case "22003":
			return fmt.Errorf("%w: %w", ErrNumericOutOfRange, err)
		case "22P02":
			return fmt.Errorf("%w: %w", ErrInvalidInputSyntax, err)
		case "42703":
			return fmt.Errorf("%w: %w", ErrUndefinedColumn, err)
		case "42P01":
			return fmt.Errorf("%w: %w", ErrUndefinedTable, err)
		default:
			if pqErr.Code.Class() == "23" {
				return fmt.Errorf("%w: %w", ErrConstraintViolation, err)
			}
			return fmt.Errorf("libdb: postgres error: code=%s detail=%q message=%q: %w",
				pqErr.Code, pqErr.Detail, pqErr.Message, err)
		}
	}

	// Wrap other unknown errors encountered (network errors, driver bugs, etc.)
	return fmt.Errorf("libdb: unexpected database error: %w", err)
}

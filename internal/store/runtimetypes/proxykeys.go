package runtimetypes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/google/uuid"
)

// ProxyKey is a minted gateway bearer's ledger row: who it was issued to and
// the window it is valid in. Only the digest (libtokenkey PurposeProxyKey hash
// of the bearer) is stored, never the bearer itself.
type ProxyKey struct {
	ID        string
	KeyHash   string
	ClientID  string
	Tier      string
	IssuedAt  time.Time
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

const proxyKeyColumns = `id, key_hash, client_id, tier, issued_at, expires_at, revoked_at, created_at`

// RecordProxyKey stores a newly minted proxy key ledger row.
func (s *store) RecordProxyKey(ctx context.Context, k ProxyKey) (*ProxyKey, error) {
	if k.KeyHash == "" || k.ClientID == "" {
		return nil, errors.New("store: proxy key requires key_hash and client_id")
	}
	if k.ID == "" {
		k.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if k.IssuedAt.IsZero() {
		k.IssuedAt = now
	}
	if k.ExpiresAt.IsZero() {
		return nil, errors.New("store: proxy key requires expires_at")
	}
	k.CreatedAt = now
	if _, err := s.ExecContext(ctx, `
		INSERT INTO proxy_keys
			(id, key_hash, client_id, tier, issued_at, expires_at, revoked_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, k.ID, k.KeyHash, k.ClientID, k.Tier, k.IssuedAt.UTC(), k.ExpiresAt.UTC(), nullTime(k.RevokedAt), k.CreatedAt); err != nil {
		return nil, fmt.Errorf("store: record proxy key: %w", err)
	}
	return &k, nil
}

// GetProxyKeyByHash returns the ledger row for a bearer digest. The lookup fails
// closed: a bearer whose digest is not recorded was never issued here.
func (s *store) GetProxyKeyByHash(ctx context.Context, keyHash string) (*ProxyKey, error) {
	if keyHash == "" {
		return nil, libdb.ErrNotFound
	}
	return scanProxyKey(s.QueryRowContext(ctx, `
		SELECT `+proxyKeyColumns+`
		FROM proxy_keys
		WHERE key_hash = $1
	`, keyHash))
}

// ListProxyKeys returns ledger rows newest created first, keyset paginated on
// created_at.
func (s *store) ListProxyKeys(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*ProxyKey, error) {
	if limit > MAXLIMIT {
		return nil, ErrLimitParamExceeded
	}
	cursor := time.Now().UTC()
	if createdAtCursor != nil {
		cursor = createdAtCursor.UTC()
	}
	rows, err := s.QueryContext(ctx, `
		SELECT `+proxyKeyColumns+`
		FROM proxy_keys
		WHERE created_at < $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list proxy keys: %w", err)
	}
	defer rows.Close()
	list := []*ProxyKey{}
	for rows.Next() {
		k, err := scanProxyKeyRows(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list proxy keys iteration: %w", err)
	}
	return list, nil
}

// RevokeProxyKey marks a minted bearer revoked, which the gateway refuses from
// then on. Idempotent: revoking an already-revoked or unknown key is a
// successful no-op that reports false.
func (s *store) RevokeProxyKey(ctx context.Context, keyHash string) (bool, error) {
	if keyHash == "" {
		return false, errors.New("store: revoke proxy key requires key_hash")
	}
	res, err := s.ExecContext(ctx, `
		UPDATE proxy_keys SET revoked_at = $1
		WHERE key_hash = $2 AND revoked_at IS NULL
	`, time.Now().UTC(), keyHash)
	if err != nil {
		return false, fmt.Errorf("store: revoke proxy key: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: revoke proxy key rows: %w", err)
	}
	return n > 0, nil
}

// CountActiveProxyKeys is how many keys are neither revoked nor expired, which
// is what an operator summary reports as live.
func (s *store) CountActiveProxyKeys(ctx context.Context, now time.Time) (int64, error) {
	var n int64
	if err := s.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM proxy_keys WHERE revoked_at IS NULL AND expires_at > $1
	`, now.UTC()).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count active proxy keys: %w", err)
	}
	return n, nil
}

func scanProxyKey(row libdb.QueryRower) (*ProxyKey, error) {
	var k ProxyKey
	var revoked sql.NullTime
	err := row.Scan(&k.ID, &k.KeyHash, &k.ClientID, &k.Tier,
		&k.IssuedAt, &k.ExpiresAt, &revoked, &k.CreatedAt)
	if err != nil {
		if errors.Is(err, libdb.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
			return nil, libdb.ErrNotFound
		}
		return nil, fmt.Errorf("store: scan proxy key: %w", err)
	}
	utcProxyKey(&k, revoked)
	return &k, nil
}

func scanProxyKeyRows(rows *sql.Rows) (*ProxyKey, error) {
	var k ProxyKey
	var revoked sql.NullTime
	if err := rows.Scan(&k.ID, &k.KeyHash, &k.ClientID, &k.Tier,
		&k.IssuedAt, &k.ExpiresAt, &revoked, &k.CreatedAt); err != nil {
		return nil, fmt.Errorf("store: scan proxy key row: %w", err)
	}
	utcProxyKey(&k, revoked)
	return &k, nil
}

func utcProxyKey(k *ProxyKey, revoked sql.NullTime) {
	k.IssuedAt = k.IssuedAt.UTC()
	k.ExpiresAt = k.ExpiresAt.UTC()
	k.CreatedAt = k.CreatedAt.UTC()
	if revoked.Valid {
		t := revoked.Time.UTC()
		k.RevokedAt = &t
	}
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

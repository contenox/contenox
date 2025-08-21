package runtimetypes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/google/uuid"
)

func (s *store) CreateRemoteHook(ctx context.Context, hook *RemoteHook) error {
	now := time.Now().UTC()
	hook.CreatedAt = now
	hook.UpdatedAt = now
	if hook.ID == "" {
		hook.ID = uuid.NewString()
	}
	_, err := s.Exec.ExecContext(ctx, `
		INSERT INTO remote_hooks
		(id, name, endpoint_url, method, timeout_ms, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		hook.ID,
		hook.Name,
		hook.EndpointURL,
		hook.Method,
		hook.TimeoutMs,
		hook.CreatedAt,
		hook.UpdatedAt,
	)
	return err
}

func (s *store) GetRemoteHook(ctx context.Context, id string) (*RemoteHook, error) {
	var hook RemoteHook
	err := s.Exec.QueryRowContext(ctx, `
		SELECT id, name, endpoint_url, method, timeout_ms, created_at, updated_at
		FROM remote_hooks
		WHERE id = $1`, id).Scan(
		&hook.ID,
		&hook.Name,
		&hook.EndpointURL,
		&hook.Method,
		&hook.TimeoutMs,
		&hook.CreatedAt,
		&hook.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, libdb.ErrNotFound
	}
	return &hook, err
}

func (s *store) GetRemoteHookByName(ctx context.Context, name string) (*RemoteHook, error) {
	var hook RemoteHook
	err := s.Exec.QueryRowContext(ctx, `
		SELECT id, name, endpoint_url, method, timeout_ms, created_at, updated_at
		FROM remote_hooks
		WHERE name = $1`, name).Scan(
		&hook.ID,
		&hook.Name,
		&hook.EndpointURL,
		&hook.Method,
		&hook.TimeoutMs,
		&hook.CreatedAt,
		&hook.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, libdb.ErrNotFound
	}
	return &hook, err
}

func (s *store) UpdateRemoteHook(ctx context.Context, hook *RemoteHook) error {
	hook.UpdatedAt = time.Now().UTC()

	result, err := s.Exec.ExecContext(ctx, `
		UPDATE remote_hooks
		SET name = $2, endpoint_url = $3, method = $4, timeout_ms = $5, updated_at = $6
		WHERE id = $1`,
		hook.ID,
		hook.Name,
		hook.EndpointURL,
		hook.Method,
		hook.TimeoutMs,
		hook.UpdatedAt,
	)

	if err != nil {
		return err
	}
	return checkRowsAffected(result)
}

func (s *store) DeleteRemoteHook(ctx context.Context, id string) error {
	result, err := s.Exec.ExecContext(ctx, `
		DELETE FROM remote_hooks
		WHERE id = $1`, id)

	if err != nil {
		return err
	}
	return checkRowsAffected(result)
}

func (s *store) ListRemoteHooks(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*RemoteHook, error) {
	cursor := time.Now().UTC()
	if createdAtCursor != nil {
		cursor = *createdAtCursor
	}
	if limit > MAXLIMIT {
		return nil, ErrLimitParamExceeded
	}

	rows, err := s.Exec.QueryContext(ctx, `
        SELECT id, name, endpoint_url, method, timeout_ms, created_at, updated_at
        FROM remote_hooks
        WHERE created_at < $1
        ORDER BY created_at DESC, id DESC
        LIMIT $2;
    `, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query remote hooks: %w", err)
	}
	defer rows.Close()

	hooks := []*RemoteHook{}
	for rows.Next() {
		var hook RemoteHook
		if err := rows.Scan(
			&hook.ID,
			&hook.Name,
			&hook.EndpointURL,
			&hook.Method,
			&hook.TimeoutMs,
			&hook.CreatedAt,
			&hook.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan remote hook: %w", err)
		}
		hooks = append(hooks, &hook)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return hooks, nil
}

func (s *store) EstimateRemoteHookCount(ctx context.Context) (int64, error) {
	return s.estimateCount(ctx, "remote_hooks")
}

package runtimetypes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	libdb "github.com/contenox/dbexec"
)

func (s *store) AppendModel(ctx context.Context, model *Model) error {
	now := time.Now().UTC()
	model.CreatedAt = now
	model.UpdatedAt = now
	if model.ContextLength <= 0 {
		return fmt.Errorf("context length cannot be zero")
	}
	_, err := s.Exec.ExecContext(ctx, `
		INSERT INTO ollama_models
		(id, model, context_length, can_chat, can_embed, can_prompt, can_stream, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		model.ID,
		model.Model,
		model.ContextLength,
		model.CanChat,
		model.CanEmbed,
		model.CanPrompt,
		model.CanStream,
		model.CreatedAt,
		model.UpdatedAt,
	)
	return err
}

func (s *store) GetModel(ctx context.Context, id string) (*Model, error) {
	var model Model
	err := s.Exec.QueryRowContext(ctx, `
        SELECT id, model, context_length, can_chat, can_embed, can_prompt, can_stream, created_at, updated_at
        FROM ollama_models
        WHERE id = $1`,
		id,
	).Scan(
		&model.ID,
		&model.Model,
		&model.ContextLength,
		&model.CanChat,
		&model.CanEmbed,
		&model.CanPrompt,
		&model.CanStream,
		&model.CreatedAt,
		&model.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, libdb.ErrNotFound
	}
	return &model, err
}

func (s *store) GetModelByName(ctx context.Context, name string) (*Model, error) {
	var model Model
	err := s.Exec.QueryRowContext(ctx, `
        SELECT id, model, context_length, can_chat, can_embed, can_prompt, can_stream, created_at, updated_at
        FROM ollama_models
        WHERE model = $1`,
		name,
	).Scan(
		&model.ID,
		&model.Model,
		&model.ContextLength,
		&model.CanChat,
		&model.CanEmbed,
		&model.CanPrompt,
		&model.CanStream,
		&model.CreatedAt,
		&model.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, libdb.ErrNotFound
	}
	return &model, err
}

func (s *store) DeleteModel(ctx context.Context, modelName string) error {
	result, err := s.Exec.ExecContext(ctx, `
		DELETE FROM ollama_models
		WHERE model = $1`,
		modelName,
	)

	if err != nil {
		return fmt.Errorf("failed to delete model: %w", err)
	}

	return checkRowsAffected(result)
}

func (s *store) ListAllModels(ctx context.Context) ([]*Model, error) {
	rows, err := s.Exec.QueryContext(ctx, `
        SELECT id, model, context_length, can_chat, can_embed, can_prompt, can_stream, created_at, updated_at
        FROM ollama_models
        ORDER BY created_at DESC, id DESC;
    `)
	if err != nil {
		return nil, fmt.Errorf("failed to query models: %w", err)
	}
	defer rows.Close()

	models := []*Model{}
	for rows.Next() {
		var model Model
		if err := rows.Scan(
			&model.ID,
			&model.Model,
			&model.ContextLength,
			&model.CanChat,
			&model.CanEmbed,
			&model.CanPrompt,
			&model.CanStream,
			&model.CreatedAt,
			&model.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan model: %w", err)
		}
		models = append(models, &model)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return models, nil
}

func (s *store) ListModels(ctx context.Context, createdAtCursor *time.Time, limit int) ([]*Model, error) {
	cursor := time.Now().UTC()
	if createdAtCursor != nil {
		cursor = *createdAtCursor
	}
	if limit > MAXLIMIT {
		return nil, ErrLimitParamExceeded
	}
	rows, err := s.Exec.QueryContext(ctx, `
        SELECT id, model, context_length, can_chat, can_embed, can_prompt, can_stream, created_at, updated_at
        FROM ollama_models
        WHERE created_at < $1
        ORDER BY created_at DESC, id DESC
        LIMIT $2;
    `, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query models: %w", err)
	}
	defer rows.Close()

	var models []*Model
	for rows.Next() {
		var model Model
		if err := rows.Scan(
			&model.ID,
			&model.Model,
			&model.ContextLength,
			&model.CanChat,
			&model.CanEmbed,
			&model.CanPrompt,
			&model.CanStream,
			&model.CreatedAt,
			&model.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan model: %w", err)
		}
		models = append(models, &model)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return models, nil
}

func (s *store) EstimateModelCount(ctx context.Context) (int64, error) {
	return s.estimateCount(ctx, "ollama_models")
}

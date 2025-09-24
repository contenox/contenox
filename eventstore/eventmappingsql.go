package eventstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

func checkRowsAffected(result sql.Result) error {
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

type mappingConfigInternal struct {
	Path               string
	EventType          string
	EventSource        string
	AggregateType      string
	AggregateIDField   string
	AggregateTypeField string
	EventTypeField     string
	EventSourceField   string
	EventIDField       string
	Version            int
	MetadataMapping    json.RawMessage
}

func newMappingConfigInternal(config *MappingConfig) (*mappingConfigInternal, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if config.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	jsonMetadataMapping, err := json.Marshal(config.MetadataMapping)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata mapping: %w", err)
	}

	return &mappingConfigInternal{
		Path:               config.Path,
		EventType:          config.EventType,
		EventSource:        config.EventSource,
		AggregateType:      config.AggregateType,
		AggregateIDField:   config.AggregateIDField,
		AggregateTypeField: config.AggregateTypeField,
		EventTypeField:     config.EventTypeField,
		EventSourceField:   config.EventSourceField,
		EventIDField:       config.EventIDField,
		Version:            config.Version,
		MetadataMapping:    jsonMetadataMapping,
	}, nil
}

func fromMappingConfigInternal(config *mappingConfigInternal) (*MappingConfig, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	var metadataMapping map[string]string
	if err := json.Unmarshal([]byte(config.MetadataMapping), &metadataMapping); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata mapping: %w", err)
	}

	if metadataMapping == nil {
		metadataMapping = make(map[string]string)
	}

	return &MappingConfig{
		Path:               config.Path,
		EventType:          config.EventType,
		EventSource:        config.EventSource,
		AggregateType:      config.AggregateType,
		AggregateIDField:   config.AggregateIDField,
		AggregateTypeField: config.AggregateTypeField,
		EventTypeField:     config.EventTypeField,
		EventSourceField:   config.EventSourceField,
		EventIDField:       config.EventIDField,
		Version:            config.Version,
		MetadataMapping:    metadataMapping,
	}, nil
}

// CreateMapping creates a new mapping config. Returns error if ID already exists.
func (s *store) CreateMapping(ctx context.Context, config *MappingConfig) error {
	internal, err := newMappingConfigInternal(config)
	if err != nil {
		return err
	}

	_, err = s.Exec.ExecContext(ctx, `
		INSERT INTO event_mappings (
			path, event_type, event_source, aggregate_type,
			aggregate_id_field, aggregate_type_field, event_type_field, event_source_field, event_id_field,
			version, metadata_mapping
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
`, internal.Path, internal.EventType, internal.EventSource, internal.AggregateType,
		internal.AggregateIDField, internal.AggregateTypeField, internal.EventTypeField, internal.EventSourceField, internal.EventIDField,
		internal.Version, internal.MetadataMapping)

	return err
}

// GetMapping retrieves a mapping config by its path
func (s *store) GetMapping(ctx context.Context, path string) (*MappingConfig, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	row := s.Exec.QueryRowContext(ctx, `
		SELECT path, event_type, event_source, aggregate_type,
		       aggregate_id_field, aggregate_type_field, event_type_field, event_source_field, event_id_field,
		       version, metadata_mapping
		FROM event_mappings
		WHERE path = $1
	`, path)

	var internal mappingConfigInternal
	err := row.Scan(
		&internal.Path,
		&internal.EventType,
		&internal.EventSource,
		&internal.AggregateType,
		&internal.AggregateIDField,
		&internal.AggregateTypeField,
		&internal.EventTypeField,
		&internal.EventSourceField,
		&internal.EventIDField,
		&internal.Version,
		&internal.MetadataMapping,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %w", ErrNotFound, err)
		}
		return nil, err
	}

	return fromMappingConfigInternal(&internal)
}

// UpdateMapping updates an existing mapping config. Returns error if not found.
func (s *store) UpdateMapping(ctx context.Context, config *MappingConfig) error {
	internal, err := newMappingConfigInternal(config)
	if err != nil {
		return err
	}

	result, err := s.Exec.ExecContext(ctx, `
		UPDATE event_mappings
		SET event_type = $1,
		    event_source = $2,
		    aggregate_type = $3,
		    aggregate_id_field = $4,
		    aggregate_type_field = $5,
		    event_type_field = $6,
		    event_source_field = $7,
		    event_id_field = $8,
		    version = $9,
		    metadata_mapping = $10
		WHERE path = $11
		`, internal.EventType, internal.EventSource, internal.AggregateType,
		internal.AggregateIDField, internal.AggregateTypeField,
		internal.EventTypeField, internal.EventSourceField, internal.EventIDField,
		internal.Version, internal.MetadataMapping, internal.Path)

	if err != nil {
		return err
	}

	return checkRowsAffected(result)
}

// DeleteMapping deletes a mapping config by path
func (s *store) DeleteMapping(ctx context.Context, path string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}

	result, err := s.Exec.ExecContext(ctx, `
		DELETE FROM event_mappings
		WHERE path = $1
	`, path)
	if err != nil {
		return err
	}

	return checkRowsAffected(result)
}

// ListMappings returns all mapping configs
func (s *store) ListMappings(ctx context.Context) ([]*MappingConfig, error) {
	rows, err := s.Exec.QueryContext(ctx, `
		SELECT path, event_type, event_source, aggregate_type,
		       aggregate_id_field, aggregate_type_field, event_type_field, event_source_field, event_id_field,
		       version, metadata_mapping
		FROM event_mappings
		ORDER BY path
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list mappings: %w", err)
	}
	defer rows.Close()

	var configs []*MappingConfig
	for rows.Next() {
		var internal mappingConfigInternal
		err := rows.Scan(
			&internal.Path,
			&internal.EventType,
			&internal.EventSource,
			&internal.AggregateType,
			&internal.AggregateIDField,
			&internal.AggregateTypeField,
			&internal.EventTypeField,
			&internal.EventSourceField,
			&internal.EventIDField,
			&internal.Version,
			&internal.MetadataMapping,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan mapping: %w", err)
		}

		config, err := fromMappingConfigInternal(&internal)
		if err != nil {
			return nil, fmt.Errorf("failed to convert internal mapping: %w", err)
		}
		configs = append(configs, config)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return configs, nil
}

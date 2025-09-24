package eventstore

import (
	"context"

	"github.com/contenox/runtime/libdbexec"
)

// InitSchema creates the main events table and initial partitions
func InitSchema(ctx context.Context, exec libdbexec.Exec) error {
	// Create main events table (partitioned)
	_, err := exec.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS events (
			id TEXT NOT NULL,
			nid BIGSERIAL NOT NULL,
			partition_key TEXT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL,
			event_type TEXT NOT NULL,
			event_source TEXT NOT NULL,
			aggregate_id TEXT NOT NULL,
			aggregate_type TEXT NOT NULL,
			version INTEGER NOT NULL,
			data JSONB NOT NULL,
			metadata JSONB,
			PRIMARY KEY (id, event_type, event_source, partition_key)
		) PARTITION BY LIST (partition_key);
	`)
	if err != nil {
		return err
	}

	// Create index on partition key for efficient querying
	_, err = exec.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_events_partition_key ON events (partition_key);
	`)
	if err != nil {
		return err
	}

	_, err = exec.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_events_created_at_brin
		ON events USING BRIN (created_at);
	`)
	if err != nil {
		return err
	}

	_, err = exec.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_events_event_type ON events (event_type);
	`)
	if err != nil {
		return err
	}

	_, err = exec.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS event_mappings (
			path TEXT PRIMARY KEY,
			event_type TEXT NOT NULL,
			event_source TEXT NOT NULL,
			aggregate_type TEXT NOT NULL,
			aggregate_id_field TEXT,
			aggregate_type_field TEXT,
			event_type_field TEXT,
			event_source_field TEXT,
			event_id_field TEXT,
			version INTEGER NOT NULL DEFAULT 1,
			metadata_mapping JSONB NOT NULL DEFAULT '{}'
		);
`)
	if err != nil {
		return err
	}

	return nil
}

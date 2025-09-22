package eventstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// generatePartitionKey creates a partition key based on event type and date
func generatePartitionKey(date time.Time) string {
	return fmt.Sprintf("%s", strings.ReplaceAll(date.Format("2006-01-02"), "-", ""))
}

// getPartitionKeysForRange generates partition keys for a date range
func getPartitionKeysForRange(from, to time.Time) []string {
	var keys []string
	current := from
	for !current.After(to) {
		keys = append(keys, generatePartitionKey(current))
		current = current.AddDate(0, 0, 1) // Add one day
	}
	return keys
}

// ErrEventTypeRequired is returned when an event type is required but not provided
var ErrEventTypeRequired = fmt.Errorf("event type is required")

// AppendEvent stores a new event in the appropriate partition
func (s *store) AppendEvent(ctx context.Context, event *Event) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}

	if event.EventType == "" {
		return ErrEventTypeRequired
	}

	// Create internal event with partition key
	internalEvent := internalEvent{
		Event:        event,
		PartitionKey: generatePartitionKey(event.CreatedAt),
	}
	if internalEvent.Metadata == nil {
		internalEvent.Metadata = []byte(`{}`)
	}
	if internalEvent.Data == nil {
		internalEvent.Data = []byte(`{}`)
	}

	// Insert the event
	_, err := s.Exec.ExecContext(ctx, `
		INSERT INTO events (id, nid, partition_key, created_at, event_type, event_source, aggregate_id, aggregate_type, version, data, metadata)
		VALUES ($1, DEFAULT, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		internalEvent.ID, internalEvent.PartitionKey, internalEvent.CreatedAt, internalEvent.EventType,
		internalEvent.EventSource, internalEvent.AggregateID, internalEvent.AggregateType, internalEvent.Version,
		internalEvent.Data, internalEvent.Metadata)

	return err
}

func (s *store) EnsurePartitionExists(ctx context.Context, ts time.Time) error {
	// Ensure the partition table exists
	if err := s.ensurePartitionTable(ctx, generatePartitionKey(ts)); err != nil {
		return fmt.Errorf("failed to ensure partition table: %w", err)
	}
	return nil
}

// ensurePartitionTable creates a partition table if it doesn't exist
func (s *store) ensurePartitionTable(ctx context.Context, partitionKey string) error {
	// Generate safe table name via hash
	tableName := getPartitionTableName(partitionKey)

	// Validate table name to prevent SQL injection
	if !isValidTableName(tableName) {
		return fmt.Errorf("invalid table name: %s", tableName)
	}

	// Escape partition key for safe use in SQL literal
	quotedKey := escapeSQLString(partitionKey)

	// Create the partition table if it doesn't exist
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s PARTITION OF events
		FOR VALUES IN (%s)
	`, tableName, quotedKey)

	_, err := s.Exec.ExecContext(ctx, query)
	return err
}

func (s *store) GetEventsByAggregate(ctx context.Context, eventType string, from, to time.Time, aggregateType, aggregateID string, limit int) ([]Event, error) {
	partitionKeys := getPartitionKeysForRange(from, to)
	if len(partitionKeys) == 0 {
		return []Event{}, nil
	}

	// Build placeholders for partition keys
	var placeholders []string
	for i := 1; i <= len(partitionKeys); i++ {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
	}
	placeholderStr := strings.Join(placeholders, ",")

	// Use subquery to force partition filtering first
	query := fmt.Sprintf(`
		SELECT id, nid, created_at, event_type, event_source, aggregate_id, aggregate_type, version, data, metadata
		FROM events
		WHERE partition_key IN (%s)
		  AND event_type = $%d
		  AND aggregate_type = $%d
		  AND aggregate_id = $%d
		  AND created_at BETWEEN $%d AND $%d
		ORDER BY created_at DESC, version DESC
		LIMIT $%d
	`,
		placeholderStr,
		len(partitionKeys)+1, // eventType
		len(partitionKeys)+2, // aggregateType
		len(partitionKeys)+3, // aggregateID
		len(partitionKeys)+4, // from
		len(partitionKeys)+5, // to
		len(partitionKeys)+6, // limit
	)

	args := make([]interface{}, 0, len(partitionKeys)+6)
	for _, key := range partitionKeys {
		args = append(args, key)
	}
	args = append(args,
		eventType,
		aggregateType,
		aggregateID,
		from,
		to,
		limit,
	)

	rows, err := s.Exec.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanEvents(rows)
}

// GetEventsByType retrieves events of a specific type within a time range
func (s *store) GetEventsByType(ctx context.Context, eventType string, from, to time.Time, limit int) ([]Event, error) {
	// Get all partition keys for the date range
	partitionKeys := getPartitionKeysForRange(from, to)

	if len(partitionKeys) == 0 {
		return []Event{}, nil
	}

	// Build $N placeholders for partition keys
	var placeholders []string
	for i := 1; i <= len(partitionKeys); i++ {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+3)) // starts after $1,$2,$3
	}
	placeholderStr := strings.Join(placeholders, ",")

	// Build query — partition_key first for pruning
	query := fmt.Sprintf(`
		SELECT id, nid, created_at, event_type, event_source, aggregate_id, aggregate_type, version, data, metadata
		FROM events
		WHERE event_type = $1
		  AND created_at BETWEEN $2 AND $3
		  AND partition_key IN (%s)
		ORDER BY created_at DESC
		LIMIT $%d
	`, placeholderStr, len(partitionKeys)+4)

	// Prepare arguments: eventType, from, to, then partition keys, then limit
	args := make([]interface{}, 0, len(partitionKeys)+4)
	args = append(args, eventType, from, to)
	for _, key := range partitionKeys {
		args = append(args, key)
	}
	args = append(args, limit)

	rows, err := s.Exec.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanEvents(rows)
}

// GetEventsBySource retrieves events from a specific source within a time range
func (s *store) GetEventsBySource(ctx context.Context, eventType string, from, to time.Time, eventSource string, limit int) ([]Event, error) {
	// Get all partition keys for the date range
	partitionKeys := getPartitionKeysForRange(from, to)

	if len(partitionKeys) == 0 {
		return []Event{}, nil
	}

	// Build $N placeholders for partition keys
	var placeholders []string
	for i := 1; i <= len(partitionKeys); i++ {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+4)) // starts after $1,$2,$3,$4
	}
	placeholderStr := strings.Join(placeholders, ",")

	// Build query — partition_key first for pruning
	query := fmt.Sprintf(`
		SELECT id, nid, created_at, event_type, event_source, aggregate_id, aggregate_type, version, data, metadata
		FROM events
		WHERE event_type = $1
		  AND event_source = $2
		  AND created_at BETWEEN $3 AND $4
		  AND partition_key IN (%s)
		ORDER BY created_at DESC
		LIMIT $%d
	`, placeholderStr, len(partitionKeys)+5)

	// Prepare arguments: eventType, eventSource, from, to, then partition keys, then limit
	args := make([]interface{}, 0, len(partitionKeys)+5)
	args = append(args, eventType, eventSource, from, to)
	for _, key := range partitionKeys {
		args = append(args, key)
	}
	args = append(args, limit)

	rows, err := s.Exec.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanEvents(rows)
}

// scanEvents helper function to scan rows into Event objects
func (s *store) scanEvents(rows *sql.Rows) ([]Event, error) {
	var events []Event

	for rows.Next() {
		var event Event
		err := rows.Scan(
			&event.ID, &event.NID, &event.CreatedAt, &event.EventType, &event.EventSource,
			&event.AggregateID, &event.AggregateType, &event.Version,
			&event.Data, &event.Metadata,
		)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(events) == 0 {
		return []Event{}, nil
	}

	return events, nil
}

func (s *store) GetEventTypesInRange(ctx context.Context, from, to time.Time, limit int) ([]string, error) {
	query := `
		SELECT DISTINCT event_type
		FROM events
		WHERE created_at BETWEEN $1 AND $2
		ORDER BY event_type
		LIMIT $3
	`

	rows, err := s.Exec.QueryContext(ctx, query, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query event types in range: %w", err)
	}
	defer rows.Close()

	var eventTypes []string
	for rows.Next() {
		var et string
		if err := rows.Scan(&et); err != nil {
			return nil, fmt.Errorf("failed to scan event type: %w", err)
		}
		eventTypes = append(eventTypes, et)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	if len(eventTypes) == 0 {
		return []string{}, nil
	}

	return eventTypes, nil
}

func (s *store) DeleteEventsByTypeInRange(ctx context.Context, eventType string, from, to time.Time) error {
	// Validate time range
	if from.After(to) {
		return fmt.Errorf("invalid time range: from %v is after to %v", from, to)
	}

	// Get all partition keys for the date range
	partitionKeys := getPartitionKeysForRange(from, to)

	if len(partitionKeys) == 0 {
		// Nothing to delete
		return nil
	}

	// Build $N placeholders for partition keys
	var placeholders []string
	for i := 1; i <= len(partitionKeys); i++ {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
	}
	placeholderStr := strings.Join(placeholders, ",")

	// Build delete query — target only relevant partitions
	query := fmt.Sprintf(`
		DELETE FROM events
		WHERE partition_key IN (%s)
		  AND event_type = $%d
		  AND created_at BETWEEN $%d AND $%d
	`,
		placeholderStr,
		len(partitionKeys)+1, // eventType
		len(partitionKeys)+2, // from
		len(partitionKeys)+3, // to
	)

	// Prepare args: partition keys, then eventType, from/to
	args := make([]interface{}, 0, len(partitionKeys)+3)
	for _, key := range partitionKeys {
		args = append(args, key)
	}
	args = append(args, eventType, from, to)

	_, err := s.Exec.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to delete events for type %s in range [%v, %v]: %w", eventType, from, to, err)
	}

	return nil
}

// getPartitionTableName generates the safe SQL table name for a partition key
func getPartitionTableName(partitionKey string) string {
	return fmt.Sprintf("events_p_%s", partitionKey)
}

// isValidTableName validates that a table name contains only safe characters
func isValidTableName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

func escapeSQLString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

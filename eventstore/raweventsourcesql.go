package eventstore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AppendRawEvent stores a new raw event in the appropriate partition
func (s *store) AppendRawEvent(ctx context.Context, event *RawEvent) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.ReceivedAt.IsZero() {
		event.ReceivedAt = time.Now().UTC()
	}

	header, err := encodeHeaders(event.Headers)
	if err != nil {
		return fmt.Errorf("failed to encode headers: %w", err)
	}
	payload, err := encodePayload(event.Payload)
	if err != nil {
		return fmt.Errorf("failed to encode payload: %w", err)
	}

	// Create internal raw event with partition key
	internal := internalRawEvent{
		ID:           event.ID,
		ReceivedAt:   event.ReceivedAt,
		Path:         event.Path,
		Headers:      header,
		Payload:      payload,
		PartitionKey: generatePartitionKey(event.ReceivedAt),
	}

	// Ensure partition exists
	if err := s.ensureRawEventPartitionTable(ctx, internal.PartitionKey); err != nil {
		return fmt.Errorf("failed to ensure raw event partition: %w", err)
	}

	// Insert the raw event and return the generated NID
	err = s.Exec.QueryRowContext(ctx, `
		INSERT INTO raw_events (
			id, nid, partition_key, received_at, path, headers, payload
		) VALUES (
			$1, DEFAULT, $2, $3, $4, $5, $6
		)
		RETURNING nid
		`,
		internal.ID,
		internal.PartitionKey,
		internal.ReceivedAt,
		internal.Path,
		internal.Headers,
		internal.Payload,
	).Scan(&internal.NID)

	event.NID = internal.NID

	return err
}

// encodeHeaders serializes headers map into a byte slice using gob
func encodeHeaders(headers map[string]string) ([]byte, error) {
	if headers == nil {
		return nil, nil
	}
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(headers); err != nil {
		return nil, fmt.Errorf("failed to gob-encode headers: %w", err)
	}
	return buf.Bytes(), nil
}

// decodeHeaders deserializes a byte slice into headers map using gob
func decodeHeaders(data []byte) (map[string]string, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var headers map[string]string
	reader := bytes.NewReader(data)
	decoder := gob.NewDecoder(reader)
	if err := decoder.Decode(&headers); err != nil {
		return nil, fmt.Errorf("failed to gob-decode headers: %w", err)
	}
	return headers, nil
}

// encodePayload serializes payload map into a byte slice using gob
func encodePayload(payload map[string]interface{}) ([]byte, error) {
	if payload == nil {
		return nil, nil
	}
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to json-encode payload: %w", err)
	}
	return jsonBytes, nil
}

// decodePayload deserializes a byte slice into payload map using gob
func decodePayload(data []byte) (map[string]interface{}, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var payload map[string]interface{}
	err := json.Unmarshal(data, &payload)
	return payload, err
}

// ensureRawEventPartitionTable creates a partition table for raw_events if it doesn't exist
func (s *store) ensureRawEventPartitionTable(ctx context.Context, partitionKey string) error {
	tableName := getRawEventPartitionTableName(partitionKey)

	if !isValidTableName(tableName) {
		return fmt.Errorf("invalid raw event table name: %s", tableName)
	}

	quotedKey := escapeSQLString(partitionKey)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s PARTITION OF raw_events
		FOR VALUES IN (%s)
	`, tableName, quotedKey)

	_, err := s.Exec.ExecContext(ctx, query)
	return err
}

// GetRawEvent retrieves a single raw event by time range and NID
func (s *store) GetRawEvent(ctx context.Context, from, to time.Time, nid int64) (*RawEvent, error) {
	partitionKeys := getPartitionKeysForRange(from, to)
	if len(partitionKeys) == 0 {
		return nil, fmt.Errorf("%w: no partitions in range", ErrNotFound)
	}

	// Build placeholders for partition keys
	var placeholders []string
	for i := 1; i <= len(partitionKeys); i++ {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
	}
	placeholderStr := strings.Join(placeholders, ",")

	query := fmt.Sprintf(`
		SELECT id, nid, received_at, path, headers, payload
		FROM raw_events
		WHERE partition_key IN (%s)
		  AND received_at BETWEEN $%d AND $%d
		  AND nid = $%d
		LIMIT 1
	`,
		placeholderStr,
		len(partitionKeys)+1, // from
		len(partitionKeys)+2, // to
		len(partitionKeys)+3, // nid
	)

	args := make([]interface{}, 0, len(partitionKeys)+3)
	for _, key := range partitionKeys {
		args = append(args, key)
	}
	args = append(args, from, to, nid)

	row := s.Exec.QueryRowContext(ctx, query, args...)

	var event RawEvent
	var headers []byte
	var payload []byte
	err := row.Scan(
		&event.ID,
		&event.NID,
		&event.ReceivedAt,
		&event.Path,
		&headers,
		&payload,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: raw event not found", ErrNotFound)
		}
		return nil, fmt.Errorf("failed to scan raw event: %w", err)
	}
	event.Headers, err = decodeHeaders(headers)
	if err != nil {
		return nil, fmt.Errorf("failed to decode headers: %w", err)
	}
	event.Payload, err = decodePayload(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	return &event, nil
}

// ListRawEvents retrieves raw events within a time range
func (s *store) ListRawEvents(ctx context.Context, from, to time.Time, limit int) ([]*RawEvent, error) {
	partitionKeys := getPartitionKeysForRange(from, to)
	if len(partitionKeys) == 0 {
		return []*RawEvent{}, nil
	}

	// Build placeholders for partition keys
	var placeholders []string
	for i := 1; i <= len(partitionKeys); i++ {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+2)) // after $1=from, $2=to
	}
	placeholderStr := strings.Join(placeholders, ",")

	query := fmt.Sprintf(`
		SELECT id, nid, received_at, path, headers, payload
		FROM raw_events
		WHERE received_at BETWEEN $1 AND $2
		  AND partition_key IN (%s)
		ORDER BY received_at DESC
		LIMIT $%d
	`, placeholderStr, len(partitionKeys)+3)

	// Args: from, to, then partition keys, then limit
	args := make([]interface{}, 0, len(partitionKeys)+3)
	args = append(args, from, to)
	for _, key := range partitionKeys {
		args = append(args, key)
	}
	args = append(args, limit)

	rows, err := s.Exec.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list raw events: %w", err)
	}
	defer rows.Close()

	var events []*RawEvent
	for rows.Next() {
		var event RawEvent
		var headers []byte
		var payload []byte

		err := rows.Scan(
			&event.ID,
			&event.NID,
			&event.ReceivedAt,
			&event.Path,
			&headers,
			&payload,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan raw event: %w", err)
		}
		event.Headers, err = decodeHeaders(headers)
		if err != nil {
			return nil, fmt.Errorf("failed to decode headers: %w", err)
		}
		event.Payload, err = decodePayload(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to decode payload: %w", err)
		}
		events = append(events, &event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return events, nil
}

// getRawEventPartitionTableName generates the safe SQL table name for a raw event partition key
func getRawEventPartitionTableName(partitionKey string) string {
	return fmt.Sprintf("raw_events_p_%s", partitionKey)
}

func (s *store) EnsureRawEventPartitionExists(ctx context.Context, ts time.Time) error {
	if err := s.ensureRawEventPartitionTable(ctx, generatePartitionKey(ts)); err != nil {
		return fmt.Errorf("failed to ensure raw event partition table: %w", err)
	}
	return nil
}

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrIdempotencyConflict = errors.New("idempotency key conflict")

func (s *Store) Idempotency(ctx context.Context, scope, key, requestHash string) ([]byte, bool, error) {
	var storedHash, response string
	err := s.db.QueryRowContext(ctx, "SELECT request_hash, response_json FROM idempotency_keys WHERE scope = ? AND key = ?", scope, key).Scan(&storedHash, &response)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if storedHash != requestHash {
		return nil, false, fmt.Errorf("%w: idempotency key already used with different request", ErrIdempotencyConflict)
	}
	return []byte(response), true, nil
}

func (s *Store) SaveIdempotency(ctx context.Context, scope, key, requestHash string, response []byte) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO idempotency_keys(scope, key, request_hash, response_json, created_at) VALUES (?, ?, ?, ?, datetime('now'))", scope, key, requestHash, string(response))
	return err
}

package store

import (
	"context"
	"database/sql"
	"errors"
)

var ErrLastAdministrator = errors.New("cannot remove or disable the last administrator")

type UserRecord struct {
	ID           string
	Username     string
	PasswordHash string
	Role         string
	AccessMode   string
	Enabled      bool
	CreatedAt    string
	UpdatedAt    string
}

type UserAPITokenRecord struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
}

type userRowScanner interface{ Scan(...any) error }

const userColumns = "id, username, password_hash, role, access_mode, enabled, created_at, updated_at"

func scanUser(row userRowScanner) (UserRecord, error) {
	var user UserRecord
	var enabled int
	if err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.AccessMode, &enabled, &user.CreatedAt, &user.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserRecord{}, ErrNotFound
		}
		return UserRecord{}, err
	}
	user.Enabled = enabled != 0
	return user, nil
}

func userEnabledInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func ensureUserRow(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func (s *Store) CountEnabledAdmins(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE role = 'admin' AND enabled = 1").Scan(&count)
	return count, err
}

func (s *Store) CreateUser(ctx context.Context, user UserRecord) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO users(id, username, password_hash, role, access_mode, enabled, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)", user.ID, user.Username, user.PasswordHash, user.Role, user.AccessMode, userEnabledInt(user.Enabled), user.CreatedAt, user.UpdatedAt)
	return err
}

func (s *Store) CreateUserWithTripEditPermissions(ctx context.Context, user UserRecord, tripIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "INSERT INTO users(id, username, password_hash, role, access_mode, enabled, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?)", user.ID, user.Username, user.PasswordHash, user.Role, user.AccessMode, userEnabledInt(user.Enabled), user.CreatedAt, user.UpdatedAt); err != nil {
		return err
	}
	for _, tripID := range tripIDs {
		if _, err := tx.ExecContext(ctx, "INSERT INTO user_trip_permissions(user_id, trip_id, can_edit, created_at) VALUES(?, ?, 1, ?)", user.ID, tripID, user.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (UserRecord, error) {
	return scanUser(s.db.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users WHERE username = ?", username))
}

func (s *Store) GetUserByID(ctx context.Context, userID string) (UserRecord, error) {
	return scanUser(s.db.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users WHERE id = ?", userID))
}

func (s *Store) ListUsers(ctx context.Context) ([]UserRecord, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+userColumns+" FROM users ORDER BY username COLLATE NOCASE")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]UserRecord, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) UpdateUser(ctx context.Context, user UserRecord, tripIDs []string, revokeAuth bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldRole string
	var oldEnabled int
	if err := tx.QueryRowContext(ctx, "SELECT role, enabled FROM users WHERE id = ?", user.ID).Scan(&oldRole, &oldEnabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if oldRole == "admin" && oldEnabled == 1 && (user.Role != "admin" || !user.Enabled) {
		var others int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE role = 'admin' AND enabled = 1 AND id <> ?", user.ID).Scan(&others); err != nil {
			return err
		}
		if others == 0 {
			return ErrLastAdministrator
		}
	}
	result, err := tx.ExecContext(ctx, "UPDATE users SET username = ?, password_hash = ?, role = ?, access_mode = ?, enabled = ?, updated_at = ? WHERE id = ?", user.Username, user.PasswordHash, user.Role, user.AccessMode, userEnabledInt(user.Enabled), user.UpdatedAt, user.ID)
	if err != nil {
		return err
	}
	if err := ensureUserRow(result); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM user_trip_permissions WHERE user_id = ?", user.ID); err != nil {
		return err
	}
	for _, tripID := range tripIDs {
		if _, err := tx.ExecContext(ctx, "INSERT INTO user_trip_permissions(user_id, trip_id, can_edit, created_at) VALUES(?, ?, 1, ?)", user.ID, tripID, user.UpdatedAt); err != nil {
			return err
		}
	}
	if revokeAuth {
		if _, err := tx.ExecContext(ctx, "DELETE FROM user_sessions WHERE user_id = ?", user.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE user_api_tokens SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL", user.UpdatedAt, user.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetUserPasswordHash(ctx context.Context, userID, hash, updatedAt string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?", hash, updatedAt, userID)
	if err != nil {
		return err
	}
	return ensureUserRow(result)
}

func (s *Store) DeleteUser(ctx context.Context, userID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var role string
	var enabled int
	if err := tx.QueryRowContext(ctx, "SELECT role, enabled FROM users WHERE id = ?", userID).Scan(&role, &enabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if role == "admin" && enabled == 1 {
		var others int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE role = 'admin' AND enabled = 1 AND id <> ?", userID).Scan(&others); err != nil {
			return err
		}
		if others == 0 {
			return ErrLastAdministrator
		}
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = ?", userID)
	if err != nil {
		return err
	}
	if err := ensureUserRow(result); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ReplaceUserTripEditPermissions(ctx context.Context, userID string, tripIDs []string, createdAt string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM user_trip_permissions WHERE user_id = ?", userID); err != nil {
		return err
	}
	for _, tripID := range tripIDs {
		if _, err := tx.ExecContext(ctx, "INSERT INTO user_trip_permissions(user_id, trip_id, can_edit, created_at) VALUES(?, ?, 1, ?)", userID, tripID, createdAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UserCanEditTrip(ctx context.Context, userID, tripID string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM user_trip_permissions WHERE user_id = ? AND trip_id = ? AND can_edit = 1", userID, tripID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) ListUserEditableTripIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT trip_id FROM user_trip_permissions WHERE user_id = ? AND can_edit = 1 ORDER BY trip_id", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) CreateUserSession(ctx context.Context, tokenHash, userID, createdAt, expiresAt string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO user_sessions(token_hash, user_id, created_at, expires_at) VALUES(?, ?, ?, ?)", tokenHash, userID, createdAt, expiresAt)
	return err
}

func (s *Store) GetUserBySessionHash(ctx context.Context, tokenHash, now string) (UserRecord, error) {
	query := "SELECT u.id, u.username, u.password_hash, u.role, u.access_mode, u.enabled, u.created_at, u.updated_at FROM user_sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = ? AND s.expires_at > ? AND u.enabled = 1"
	return scanUser(s.db.QueryRowContext(ctx, query, tokenHash, now))
}

func (s *Store) DeleteUserSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM user_sessions WHERE token_hash = ?", tokenHash)
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM user_sessions WHERE user_id = ?", userID)
	return err
}

func (s *Store) CreateUserAPIToken(ctx context.Context, tokenID, tokenHash, userID, name, createdAt string, expiresAt *string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO user_api_tokens(id, token_hash, user_id, name, created_at, expires_at) VALUES(?, ?, ?, ?, ?, ?)", tokenID, tokenHash, userID, name, createdAt, expiresAt)
	return err
}

func (s *Store) GetUserByAPITokenHash(ctx context.Context, tokenHash, now string) (UserRecord, error) {
	query := "SELECT u.id, u.username, u.password_hash, u.role, u.access_mode, u.enabled, u.created_at, u.updated_at FROM user_api_tokens t JOIN users u ON u.id = t.user_id WHERE t.token_hash = ? AND t.revoked_at IS NULL AND (t.expires_at IS NULL OR t.expires_at > ?) AND u.enabled = 1"
	return scanUser(s.db.QueryRowContext(ctx, query, tokenHash, now))
}

func (s *Store) TouchUserAPIToken(ctx context.Context, tokenHash, usedAt string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE user_api_tokens SET last_used_at = ? WHERE token_hash = ? AND revoked_at IS NULL", usedAt, tokenHash)
	return err
}

func (s *Store) ListUserAPITokens(ctx context.Context, userID string) ([]UserAPITokenRecord, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name, created_at, COALESCE(last_used_at, ''), COALESCE(expires_at, '') FROM user_api_tokens WHERE user_id = ? AND revoked_at IS NULL ORDER BY created_at DESC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tokens := make([]UserAPITokenRecord, 0)
	for rows.Next() {
		var token UserAPITokenRecord
		if err := rows.Scan(&token.ID, &token.Name, &token.CreatedAt, &token.LastUsedAt, &token.ExpiresAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

func (s *Store) RevokeUserAPIToken(ctx context.Context, userID, tokenID, revokedAt string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE user_api_tokens SET revoked_at = ? WHERE user_id = ? AND id = ? AND revoked_at IS NULL", revokedAt, userID, tokenID)
	if err != nil {
		return err
	}
	return ensureUserRow(result)
}

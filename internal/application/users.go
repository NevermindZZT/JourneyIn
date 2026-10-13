package application

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"journeyin/internal/security"
	"journeyin/internal/store"
)

var ErrUsernameTaken = errors.New("username already exists")

const (
	passwordHashIterations = 600000
	passwordSaltBytes      = 16
	passwordKeyBytes       = 32
	minimumPasswordRunes   = 12
	userSessionLifetime    = 24 * time.Hour
)

type ManagedUser struct {
	ID              string   `json:"id"`
	Username        string   `json:"username"`
	Role            string   `json:"role"`
	AccessMode      string   `json:"access_mode"`
	Enabled         bool     `json:"enabled"`
	EditableTripIDs []string `json:"editable_trip_ids"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

type UserService struct{ store *store.Store }

func NewUserService(database *store.Store) *UserService { return &UserService{store: database} }

func (s *UserService) BootstrapAdminIfEmpty(ctx context.Context, username, password string) (bool, error) {
	if s == nil || s.store == nil {
		return false, errors.New("user store is not configured")
	}
	username = strings.TrimSpace(username)
	if username == "" && password == "" {
		return false, nil
	}
	if username == "" || strings.TrimSpace(password) == "" {
		return false, errors.New("both bootstrap username and a non-blank password are required")
	}
	count, err := s.store.UserCount(ctx)
	if err != nil {
		return false, err
	}
	if count != 0 {
		return false, nil
	}
	hash, err := hashUserPassword(password)
	if err != nil {
		return false, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id, err := newUserID()
	if err != nil {
		return false, err
	}
	user := store.UserRecord{ID: id, Username: strings.TrimSpace(username), PasswordHash: hash, Role: string(security.RoleAdmin), AccessMode: string(security.AccessAllTripsEdit), Enabled: true, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateUser(ctx, user); err != nil {
		return false, err
	}
	return true, nil
}

func (s *UserService) HasUsers(ctx context.Context) (bool, error) {
	count, err := s.store.UserCount(ctx)
	return count > 0, err
}

func (s *UserService) Authenticate(ctx context.Context, username, password string) (security.Principal, error) {
	user, err := s.store.GetUserByUsername(ctx, strings.TrimSpace(username))
	if err != nil || !user.Enabled || !verifyUserPassword(user.PasswordHash, password) {
		return security.Principal{}, errors.New("invalid username or password")
	}
	return principalForUser(user), nil
}

func (s *UserService) CreateSession(ctx context.Context, userID string) (string, time.Time, error) {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil || !user.Enabled {
		return "", time.Time{}, errors.New("user is disabled or unavailable")
	}
	raw, err := randomToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now().UTC()
	expires := now.Add(userSessionLifetime)
	if err := s.store.CreateUserSession(ctx, tokenDigest(raw), userID, now.Format(time.RFC3339), expires.Format(time.RFC3339)); err != nil {
		return "", time.Time{}, err
	}
	return raw, expires, nil
}

func (s *UserService) ResolveSession(ctx context.Context, raw string) (security.Principal, error) {
	if raw == "" {
		return security.Principal{}, store.ErrNotFound
	}
	user, err := s.store.GetUserBySessionHash(ctx, tokenDigest(raw), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return security.Principal{}, err
	}
	return principalForUser(user), nil
}

func (s *UserService) RevokeSession(ctx context.Context, raw string) error {
	if raw == "" {
		return nil
	}
	return s.store.DeleteUserSession(ctx, tokenDigest(raw))
}

func (s *UserService) CreateUser(ctx context.Context, actor security.Principal, username, password, role, accessMode string, editableTripIDs []string) (ManagedUser, error) {
	if !actor.IsAdmin() {
		return ManagedUser{}, errors.New("administrator permission is required")
	}
	username = strings.TrimSpace(username)
	if username == "" || utf8.RuneCountInString(username) > 80 {
		return ManagedUser{}, errors.New("username must be 1 to 80 characters")
	}
	if _, err := s.store.GetUserByUsername(ctx, username); err == nil {
		return ManagedUser{}, ErrUsernameTaken
	} else if !errors.Is(err, store.ErrNotFound) {
		return ManagedUser{}, err
	}
	if strings.TrimSpace(password) == "" || utf8.RuneCountInString(password) < minimumPasswordRunes {
		return ManagedUser{}, fmt.Errorf("password must be at least %d characters", minimumPasswordRunes)
	}
	userCount, err := s.store.UserCount(ctx)
	if err != nil {
		return ManagedUser{}, err
	}
	if role != string(security.RoleAdmin) && role != string(security.RoleUser) {
		return ManagedUser{}, errors.New("role must be admin or user")
	}
	if userCount == 0 && role != string(security.RoleAdmin) {
		return ManagedUser{}, errors.New("the first user must be an administrator")
	}
	if role == string(security.RoleAdmin) {
		accessMode = string(security.AccessAllTripsEdit)
	}
	if !validAccessMode(accessMode) {
		return ManagedUser{}, errors.New("invalid access_mode")
	}
	editableTripIDs = uniqueStrings(editableTripIDs)
	if role != string(security.RoleAdmin) && accessMode != string(security.AccessSelectedTripEdit) && len(editableTripIDs) > 0 {
		return ManagedUser{}, errors.New("editable_trip_ids are allowed only for selected_trip_edit")
	}
	hash, err := hashUserPassword(password)
	if err != nil {
		return ManagedUser{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id, err := newUserID()
	if err != nil {
		return ManagedUser{}, err
	}
	user := store.UserRecord{ID: id, Username: username, PasswordHash: hash, Role: role, AccessMode: accessMode, Enabled: true, CreatedAt: now, UpdatedAt: now}
	if role == string(security.RoleAdmin) || accessMode != string(security.AccessSelectedTripEdit) {
		editableTripIDs = nil
	}
	if err := s.store.CreateUserWithTripEditPermissions(ctx, user, editableTripIDs); err != nil {
		return ManagedUser{}, err
	}
	return s.managedUser(ctx, user)
}

func (s *UserService) ListUsers(ctx context.Context, actor security.Principal) ([]ManagedUser, error) {
	if !actor.IsAdmin() {
		return nil, errors.New("administrator permission is required")
	}
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]ManagedUser, 0, len(users))
	for _, user := range users {
		profile, err := s.managedUser(ctx, user)
		if err != nil {
			return nil, err
		}
		result = append(result, profile)
	}
	return result, nil
}

func (s *UserService) UpdateUser(ctx context.Context, actor security.Principal, userID, username, role, accessMode string, enabled bool, editableTripIDs []string, newPassword string) (ManagedUser, error) {
	if !actor.IsAdmin() {
		return ManagedUser{}, errors.New("administrator permission is required")
	}
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return ManagedUser{}, err
	}
	username = strings.TrimSpace(username)
	if username == "" || utf8.RuneCountInString(username) > 80 {
		return ManagedUser{}, errors.New("username must be 1 to 80 characters")
	}
	if existing, err := s.store.GetUserByUsername(ctx, username); err == nil && existing.ID != user.ID {
		return ManagedUser{}, ErrUsernameTaken
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return ManagedUser{}, err
	}
	if role != string(security.RoleAdmin) && role != string(security.RoleUser) {
		return ManagedUser{}, errors.New("role must be admin or user")
	}
	if role == string(security.RoleAdmin) {
		accessMode = string(security.AccessAllTripsEdit)
	}
	if !validAccessMode(accessMode) {
		return ManagedUser{}, errors.New("invalid access_mode")
	}
	if user.Role == string(security.RoleAdmin) && user.Enabled && (role != string(security.RoleAdmin) || !enabled) {
		count, err := s.store.CountEnabledAdmins(ctx)
		if err != nil {
			return ManagedUser{}, err
		}
		if count <= 1 {
			return ManagedUser{}, store.ErrLastAdministrator
		}
	}
	editableTripIDs = uniqueStrings(editableTripIDs)
	if role != string(security.RoleAdmin) && accessMode != string(security.AccessSelectedTripEdit) && len(editableTripIDs) > 0 {
		return ManagedUser{}, errors.New("editable_trip_ids are allowed only for selected_trip_edit")
	}
	user.Username, user.Role, user.AccessMode, user.Enabled, user.UpdatedAt = username, role, accessMode, enabled, time.Now().UTC().Format(time.RFC3339Nano)
	if newPassword != "" {
		if strings.TrimSpace(newPassword) == "" || utf8.RuneCountInString(newPassword) < minimumPasswordRunes {
			return ManagedUser{}, fmt.Errorf("password must be at least %d characters", minimumPasswordRunes)
		}
		hash, err := hashUserPassword(newPassword)
		if err != nil {
			return ManagedUser{}, err
		}
		user.PasswordHash = hash
	}
	if role == string(security.RoleAdmin) || accessMode != string(security.AccessSelectedTripEdit) {
		editableTripIDs = nil
	}
	revokeAuth := !enabled || newPassword != ""
	if err := s.store.UpdateUser(ctx, user, editableTripIDs, revokeAuth); err != nil {
		return ManagedUser{}, err
	}
	return s.managedUser(ctx, user)
}

func (s *UserService) DeleteUser(ctx context.Context, actor security.Principal, userID string) error {
	if !actor.IsAdmin() {
		return errors.New("administrator permission is required")
	}
	if actor.UserID == userID {
		return errors.New("cannot delete the current administrator account")
	}
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Role == string(security.RoleAdmin) && user.Enabled {
		count, err := s.store.CountEnabledAdmins(ctx)
		if err != nil {
			return err
		}
		if count <= 1 {
			return store.ErrLastAdministrator
		}
	}
	return s.store.DeleteUser(ctx, userID)
}

func (s *UserService) PrincipalForSession(ctx context.Context, raw string) (security.Principal, error) {
	user, err := s.store.GetUserBySessionHash(ctx, tokenDigest(raw), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return security.Principal{}, err
	}
	return principalFromRecord(user), nil
}

func (s *UserService) CreateAPIToken(ctx context.Context, actor security.Principal, userID, name string) (string, error) {
	if !actor.IsAdmin() {
		return "", errors.New("administrator permission is required")
	}
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil || !user.Enabled {
		return "", errors.New("user is disabled or unavailable")
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 80 {
		return "", errors.New("token name must be 1 to 80 characters")
	}
	raw, err := randomToken(32)
	if err != nil {
		return "", err
	}
	tokenID, err := newUserID()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	expires := now.Add(90 * 24 * time.Hour).Format(time.RFC3339)
	if err := s.store.CreateUserAPIToken(ctx, tokenID, tokenDigest(raw), userID, name, now.Format(time.RFC3339), &expires); err != nil {
		return "", err
	}
	return raw, nil
}

func (s *UserService) PrincipalForAPIToken(ctx context.Context, raw string) (security.Principal, error) {
	if raw == "" {
		return security.Principal{}, store.ErrNotFound
	}
	user, err := s.store.GetUserByAPITokenHash(ctx, tokenDigest(raw), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return security.Principal{}, err
	}
	_ = s.store.TouchUserAPIToken(ctx, tokenDigest(raw), time.Now().UTC().Format(time.RFC3339))
	return principalFromRecord(user), nil
}

func (s *UserService) ListAPITokens(ctx context.Context, actor security.Principal, userID string) ([]store.UserAPITokenRecord, error) {
	if !actor.IsAdmin() {
		return nil, errors.New("administrator permission is required")
	}
	return s.store.ListUserAPITokens(ctx, userID)
}

func (s *UserService) RevokeAPIToken(ctx context.Context, actor security.Principal, userID, tokenID string) error {
	if !actor.IsAdmin() {
		return errors.New("administrator permission is required")
	}
	return s.store.RevokeUserAPIToken(ctx, userID, tokenID, time.Now().UTC().Format(time.RFC3339))
}

func (s *UserService) CanEditTrip(ctx context.Context, principal security.Principal, tripID string) (bool, error) {
	if principal.IsAdmin() || principal.AccessMode == security.AccessAllTripsEdit {
		return true, nil
	}
	if principal.AccessMode != security.AccessSelectedTripEdit || principal.UserID == "" {
		return false, nil
	}
	return s.store.UserCanEditTrip(ctx, principal.UserID, tripID)
}

func (s *UserService) CanCreateTrip(principal security.Principal) bool {
	return principal.IsAdmin() || principal.AccessMode == security.AccessAllTripsEdit
}

func (s *UserService) IsAdmin(principal security.Principal) bool { return principal.IsAdmin() }

func (s *UserService) managedUser(ctx context.Context, user store.UserRecord) (ManagedUser, error) {
	tripIDs, err := s.store.ListUserEditableTripIDs(ctx, user.ID)
	if err != nil {
		return ManagedUser{}, err
	}
	return ManagedUser{ID: user.ID, Username: user.Username, Role: user.Role, AccessMode: user.AccessMode, Enabled: user.Enabled, EditableTripIDs: tripIDs, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt}, nil
}

func principalFromRecord(user store.UserRecord) security.Principal {
	return security.Principal{UserID: user.ID, Username: user.Username, Role: security.Role(user.Role), AccessMode: security.AccessMode(user.AccessMode)}
}

func principalForUser(user store.UserRecord) security.Principal { return principalFromRecord(user) }

func validAccessMode(mode string) bool {
	return mode == string(security.AccessReadOnly) || mode == string(security.AccessSelectedTripEdit) || mode == string(security.AccessAllTripsEdit)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func randomToken(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func tokenDigest(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func hashUserPassword(password string) (string, error) {
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	derived, err := pbkdf2.Key(sha256.New, password, salt, passwordHashIterations, passwordKeyBytes)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", passwordHashIterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(derived)), nil
}

func verifyUserPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	var iterations int
	if _, err := fmt.Sscanf(parts[1], "%d", &iterations); err != nil || iterations < 100000 || iterations > 2000000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(expected) != passwordKeyBytes {
		return false
	}
	actual, err := pbkdf2.Key(sha256.New, password, salt, iterations, passwordKeyBytes)
	return err == nil && subtle.ConstantTimeCompare(expected, actual) == 1
}

func newUserID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

package httpapi

import (
	"context"
	"errors"
	"net/http"

	"journeyin/internal/application"
	"journeyin/internal/security"
	"journeyin/internal/store"
)

type createUserRequest struct {
	Username        string   `json:"username"`
	Password        string   `json:"password"`
	Role            string   `json:"role"`
	AccessMode      string   `json:"access_mode"`
	EditableTripIDs []string `json:"editable_trip_ids"`
}

type updateUserRequest struct {
	Username        string   `json:"username"`
	Role            string   `json:"role"`
	AccessMode      string   `json:"access_mode"`
	Enabled         *bool    `json:"enabled"`
	EditableTripIDs []string `json:"editable_trip_ids"`
	NewPassword     string   `json:"new_password,omitempty"`
}

type createUserTokenRequest struct {
	Name string `json:"name"`
}

func (s *Server) currentUser(w http.ResponseWriter, r *http.Request) {
	principal, ok := security.PrincipalFromContext(r.Context())
	if !ok {
		principal = security.Principal{UserID: "system", Username: "Local administrator", Role: security.RoleAdmin, AccessMode: security.AccessAllTripsEdit, System: true}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": principal.UserID, "username": principal.Username, "role": principal.Role,
		"access_mode": principal.AccessMode, "is_admin": principal.IsAdmin(),
	})
}

func (s *Server) canEditTrip(ctx context.Context, tripID string) bool {
	principal, ok := security.PrincipalFromContext(ctx)
	if !ok || principal.IsAdmin() || principal.AccessMode == security.AccessAllTripsEdit {
		return true
	}
	if s.userService == nil {
		return false
	}
	allowed, err := s.userService.CanEditTrip(ctx, principal, tripID)
	return err == nil && allowed
}

func (s *Server) requireAdminPrincipal(w http.ResponseWriter, r *http.Request) (security.Principal, bool) {
	principal, ok := security.PrincipalFromContext(r.Context())
	if !ok {
		principal = security.SystemPrincipal()
	}
	if !principal.IsAdmin() {
		writeError(w, http.StatusForbidden, "forbidden", "administrator permission is required", nil)
		return security.Principal{}, false
	}
	return principal, true
}

func (s *Server) requireUserAdmin(w http.ResponseWriter, r *http.Request) (security.Principal, bool) {
	principal, ok := s.requireAdminPrincipal(w, r)
	if !ok {
		return security.Principal{}, false
	}
	if s.userService == nil {
		writeError(w, http.StatusServiceUnavailable, "user_management_unavailable", "user management is not configured", nil)
		return security.Principal{}, false
	}
	return principal, true
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requireUserAdmin(w, r)
	if !ok {
		return
	}
	users, err := s.userService.ListUsers(r.Context(), principal)
	if err != nil {
		writeUserError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": users})
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requireUserAdmin(w, r)
	if !ok {
		return
	}
	var body createUserRequest
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	user, err := s.userService.CreateUser(r.Context(), principal, body.Username, body.Password, body.Role, body.AccessMode, body.EditableTripIDs)
	if err != nil {
		writeUserError(w, err)
		return
	}
	if s.auth != nil {
		s.auth.RefreshUserModeEnabled()
	}
	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requireUserAdmin(w, r)
	if !ok {
		return
	}
	var body updateUserRequest
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if body.Enabled == nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "enabled is required", nil)
		return
	}
	user, err := s.userService.UpdateUser(r.Context(), principal, r.PathValue("userID"), body.Username, body.Role, body.AccessMode, *body.Enabled, body.EditableTripIDs, body.NewPassword)
	if err != nil {
		writeUserError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requireUserAdmin(w, r)
	if !ok {
		return
	}
	if err := s.userService.DeleteUser(r.Context(), principal, r.PathValue("userID")); err != nil {
		writeUserError(w, err)
		return
	}
	if s.auth != nil {
		s.auth.RefreshUserModeEnabled()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listUserTokens(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requireUserAdmin(w, r)
	if !ok {
		return
	}
	tokens, err := s.userService.ListAPITokens(r.Context(), principal, r.PathValue("userID"))
	if err != nil {
		writeUserError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": tokens})
}

func (s *Server) createUserToken(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requireUserAdmin(w, r)
	if !ok {
		return
	}
	var body createUserTokenRequest
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	raw, err := s.userService.CreateAPIToken(r.Context(), principal, r.PathValue("userID"), body.Name)
	if err != nil {
		writeUserError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": raw, "message": "Copy this token now; it will not be shown again."})
}

func (s *Server) revokeUserToken(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requireUserAdmin(w, r)
	if !ok {
		return
	}
	if err := s.userService.RevokeAPIToken(r.Context(), principal, r.PathValue("userID"), r.PathValue("tokenID")); err != nil {
		writeUserError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writePermissionError(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, security.ErrForbidden) {
		return false
	}
	writeError(w, http.StatusForbidden, "forbidden", "this account cannot edit the requested trip", nil)
	return true
}

func writeUserError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, security.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "administrator permission is required", nil)
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "user or token not found", nil)
	case errors.Is(err, store.ErrLastAdministrator):
		writeError(w, http.StatusConflict, "last_administrator", "cannot remove or disable the last administrator", nil)
	case errors.Is(err, application.ErrUsernameTaken):
		writeError(w, http.StatusConflict, "username_taken", "that username is already in use", nil)
	default:
		writeError(w, http.StatusBadRequest, "user_error", err.Error(), nil)
	}
}

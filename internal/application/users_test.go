package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"journeyin/internal/security"
	"journeyin/internal/store"
)

func testUserService(t *testing.T) (*TripService, *UserService) {
	t.Helper()
	trips := testService(t)
	users := NewUserService(trips.store)
	trips.SetUserService(users)
	return trips, users
}

func TestFirstPersistentUserMustBeAdministrator(t *testing.T) {
	ctx := context.Background()
	_, users := testUserService(t)
	if _, err := users.CreateUser(ctx, security.SystemPrincipal(), "first-user", "first-user-long-password", "user", string(security.AccessReadOnly), nil); err == nil {
		t.Fatal("created a non-admin first account")
	}
	if _, err := users.CreateUser(ctx, security.SystemPrincipal(), "blank-password", "            ", "admin", string(security.AccessAllTripsEdit), nil); err == nil {
		t.Fatal("accepted an all-whitespace password")
	}
	created, err := users.CreateUser(ctx, security.SystemPrincipal(), "first-admin", "first-admin-long-password", "admin", string(security.AccessAllTripsEdit), nil)
	if err != nil || created.Role != string(security.RoleAdmin) {
		t.Fatalf("create first admin: profile=%+v err=%v", created, err)
	}
}

func TestBootstrapUserPermissionsAndTripGuards(t *testing.T) {
	ctx := context.Background()
	trips, users := testUserService(t)
	created, err := users.BootstrapAdminIfEmpty(ctx, "root", "legacy-admin-password")
	if err != nil || !created {
		t.Fatalf("bootstrap admin: created=%v err=%v", created, err)
	}
	admin, err := users.Authenticate(ctx, "root", "legacy-admin-password")
	if err != nil || !admin.IsAdmin() {
		t.Fatalf("authenticate admin: principal=%+v err=%v", admin, err)
	}
	stored, err := trips.store.GetUserByUsername(ctx, "root")
	if err != nil || stored.PasswordHash == "legacy-admin-password" || !strings.HasPrefix(stored.PasswordHash, "pbkdf2-sha256$") {
		t.Fatalf("password was not stored as a PBKDF2 hash: %+v err=%v", stored, err)
	}

	first, err := trips.Create(ctx, []byte(validTrip), "test")
	if err != nil {
		t.Fatal(err)
	}
	second, err := trips.Create(ctx, []byte(strings.Replace(validTrip, "测试行程", "第二条行程", 1)), "test")
	if err != nil {
		t.Fatal(err)
	}
	selectedUser, err := users.CreateUser(ctx, admin, "selected-editor", "a-long-enough-user-password", "user", string(security.AccessSelectedTripEdit), []string{first.ID})
	if err != nil {
		t.Fatal(err)
	}
	selectedPrincipal, err := users.Authenticate(ctx, selectedUser.Username, "a-long-enough-user-password")
	if err != nil {
		t.Fatal(err)
	}
	selectedCtx := security.WithPrincipal(ctx, selectedPrincipal)
	if _, err := trips.Get(selectedCtx, second.ID); err != nil {
		t.Fatalf("selected editor should read all trips: %v", err)
	}
	changed := []byte(strings.Replace(validTrip, "测试行程", "授权编辑", 1))
	if _, err := trips.Replace(selectedCtx, first.ID, first.Revision, changed, "test"); err != nil {
		t.Fatalf("selected editor could not edit granted trip: %v", err)
	}
	if _, err := trips.Replace(selectedCtx, second.ID, second.Revision, changed, "test"); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("edit of ungranted trip = %v, want forbidden", err)
	}
	if _, err := trips.Create(selectedCtx, []byte(validTrip), "test"); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("selected editor create = %v, want forbidden", err)
	}

	readOnly, err := users.CreateUser(ctx, admin, "viewer", "a-different-long-password", "user", string(security.AccessReadOnly), nil)
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := users.Authenticate(ctx, readOnly.Username, "a-different-long-password")
	if err != nil {
		t.Fatal(err)
	}
	viewerCtx := security.WithPrincipal(ctx, viewer)
	if _, err := trips.Get(viewerCtx, first.ID); err != nil {
		t.Fatalf("read-only user should read trips: %v", err)
	}
	if _, err := trips.Replace(viewerCtx, first.ID, first.Revision+1, changed, "test"); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("read-only edit = %v, want forbidden", err)
	}

	allEditor, err := users.CreateUser(ctx, admin, "all-editor", "another-long-user-password", "user", string(security.AccessAllTripsEdit), nil)
	if err != nil {
		t.Fatal(err)
	}
	allPrincipal, err := users.Authenticate(ctx, allEditor.Username, "another-long-user-password")
	if err != nil {
		t.Fatal(err)
	}
	if !users.CanCreateTrip(allPrincipal) {
		t.Fatal("all-trip editor should be able to create trips")
	}
	if ok, err := users.CanEditTrip(ctx, allPrincipal, second.ID); err != nil || !ok {
		t.Fatalf("all-trip editor edit access = %v, %v", ok, err)
	}
}

func TestSavePreviewCannotBeCommittedByAnotherUser(t *testing.T) {
	ctx := context.Background()
	trips, users := testUserService(t)
	if created, err := users.BootstrapAdminIfEmpty(ctx, "root", "bootstrap-password"); err != nil || !created {
		t.Fatalf("bootstrap: created=%v err=%v", created, err)
	}
	admin, err := users.Authenticate(ctx, "root", "bootstrap-password")
	if err != nil {
		t.Fatal(err)
	}
	trip, err := trips.Create(ctx, []byte(validTrip), "test")
	if err != nil {
		t.Fatal(err)
	}
	first, err := users.CreateUser(ctx, admin, "preview-owner", "preview-owner-password", "user", string(security.AccessSelectedTripEdit), []string{trip.ID})
	if err != nil {
		t.Fatal(err)
	}
	second, err := users.CreateUser(ctx, admin, "other-editor", "other-editor-password", "user", string(security.AccessAllTripsEdit), nil)
	if err != nil {
		t.Fatal(err)
	}
	firstPrincipal, err := users.Authenticate(ctx, first.Username, "preview-owner-password")
	if err != nil {
		t.Fatal(err)
	}
	secondPrincipal, err := users.Authenticate(ctx, second.Username, "other-editor-password")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := trips.PreviewSave(security.WithPrincipal(ctx, firstPrincipal), []byte(validTrip), "replace", trip.ID, trip.Revision, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := trips.CommitSave(security.WithPrincipal(ctx, secondPrincipal), preview.PreviewID, preview.ConfirmationToken, "other-user-key", trip.Revision, "test"); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("another user commit = %v, want forbidden", err)
	}
	if _, err := trips.CommitSave(security.WithPrincipal(ctx, firstPrincipal), preview.PreviewID, preview.ConfirmationToken, "owner-user-key", trip.Revision, "test"); err != nil {
		t.Fatalf("preview owner could not commit: %v", err)
	}
}

func TestUserSessionsAndTokensRevokeOnPasswordReset(t *testing.T) {
	ctx := context.Background()
	trips, users := testUserService(t)
	if created, err := users.BootstrapAdminIfEmpty(ctx, "root", "bootstrap-password"); err != nil || !created {
		t.Fatalf("bootstrap: created=%v err=%v", created, err)
	}
	admin, err := users.Authenticate(ctx, "root", "bootstrap-password")
	if err != nil {
		t.Fatal(err)
	}
	user, err := users.CreateUser(ctx, admin, "agent-user", "initial-long-password", "user", string(security.AccessReadOnly), nil)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := users.CreateSession(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if principal, err := users.PrincipalForSession(ctx, token); err != nil || principal.UserID != user.ID {
		t.Fatalf("resolve session: principal=%+v err=%v", principal, err)
	}
	apiToken, err := users.CreateAPIToken(ctx, admin, user.ID, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if principal, err := users.PrincipalForAPIToken(ctx, apiToken); err != nil || principal.UserID != user.ID || principal.AccessMode != security.AccessReadOnly {
		t.Fatalf("resolve API token: principal=%+v err=%v", principal, err)
	}
	metadata, err := users.ListAPITokens(ctx, admin, user.ID)
	if err != nil || len(metadata) != 1 || metadata[0].Name != "agent" {
		t.Fatalf("token metadata=%+v err=%v", metadata, err)
	}
	if _, err := users.UpdateUser(ctx, admin, user.ID, user.Username, user.Role, user.AccessMode, true, nil, "replacement-password-long"); err != nil {
		t.Fatal(err)
	}
	if _, err := users.PrincipalForSession(ctx, token); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("old session remains valid: %v", err)
	}
	if _, err := users.PrincipalForAPIToken(ctx, apiToken); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("old API token remains valid: %v", err)
	}
	if _, err := users.Authenticate(ctx, user.Username, "initial-long-password"); err == nil {
		t.Fatal("old password still authenticates")
	}
	if _, err := users.Authenticate(ctx, user.Username, "replacement-password-long"); err != nil {
		t.Fatalf("new password does not authenticate: %v", err)
	}
	_ = trips
}

func TestLastAdministratorCannotBeDeletedOrDemoted(t *testing.T) {
	ctx := context.Background()
	_, users := testUserService(t)
	if _, err := users.BootstrapAdminIfEmpty(ctx, "root", "bootstrap-password"); err != nil {
		t.Fatal(err)
	}
	admin, err := users.Authenticate(ctx, "root", "bootstrap-password")
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := users.ListUsers(ctx, admin)
	if err != nil || len(profiles) != 1 {
		t.Fatalf("profiles=%+v err=%v", profiles, err)
	}
	if err := users.DeleteUser(ctx, admin, admin.UserID); err == nil {
		t.Fatal("deleted the current/last administrator")
	}
	if _, err := users.UpdateUser(ctx, admin, admin.UserID, "root", "user", string(security.AccessReadOnly), false, nil, ""); !errors.Is(err, store.ErrLastAdministrator) {
		t.Fatalf("demote last admin = %v, want last-admin denial", err)
	}
}

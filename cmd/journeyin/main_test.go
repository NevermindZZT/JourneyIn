package main

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"

	journeyin "journeyin"
	"journeyin/internal/application"
	"journeyin/internal/security"
	"journeyin/internal/store"
	"journeyin/internal/transport/httpapi"
)

func TestIsLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8080", "localhost:8080", "[::1]:8080"} {
		if !isLoopback(addr) {
			t.Errorf("isLoopback(%q)=false, want true", addr)
		}
	}
	for _, addr := range []string{"0.0.0.0:8080", "journeyin.example:8080", "not-an-address"} {
		if isLoopback(addr) {
			t.Errorf("isLoopback(%q)=true, want false", addr)
		}
	}
}

func TestResolveStdioPrincipalRequiresScopedTokenForPersistentUsers(t *testing.T) {
	ctx := context.Background()
	localAuth := httpapi.NewAuthenticator("", "", "")
	principal, err := resolveStdioPrincipal(ctx, localAuth, "", "")
	if err != nil || !principal.IsAdmin() {
		t.Fatalf("local no-auth stdio principal=%+v err=%v", principal, err)
	}

	rootAuth := httpapi.NewAuthenticator("", "", "legacy-api-token")
	if _, err := resolveStdioPrincipal(ctx, rootAuth, "", ""); err == nil {
		t.Fatal("stdio accepted an empty token when auth is enabled")
	}
	principal, err = resolveStdioPrincipal(ctx, rootAuth, "", "legacy-api-token")
	if err != nil || !principal.IsAdmin() {
		t.Fatalf("legacy token stdio principal=%+v err=%v", principal, err)
	}

	migrations, err := fs.Sub(journeyin.MigrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "stdio.db"), migrations)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	users := application.NewUserService(database)
	if created, err := users.BootstrapAdminIfEmpty(ctx, "root", "bootstrap-password"); err != nil || !created {
		t.Fatalf("bootstrap: created=%v err=%v", created, err)
	}
	viewer, err := users.CreateUser(ctx, security.SystemPrincipal(), "stdio-viewer", "stdio-viewer-password", "user", string(security.AccessReadOnly), nil)
	if err != nil {
		t.Fatal(err)
	}
	token, err := users.CreateAPIToken(ctx, security.SystemPrincipal(), viewer.ID, "stdio")
	if err != nil {
		t.Fatal(err)
	}
	auth := httpapi.NewAuthenticator("", "", "")
	auth.SetUserService(users)
	principal, err = resolveStdioPrincipal(ctx, auth, "", token)
	if err != nil || principal.UserID != viewer.ID || principal.AccessMode != security.AccessReadOnly {
		t.Fatalf("scoped stdio principal=%+v err=%v", principal, err)
	}
}

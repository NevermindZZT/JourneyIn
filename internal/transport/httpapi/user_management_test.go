package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	journeyin "journeyin"
	"journeyin/internal/application"
	"journeyin/internal/security"
	journeyshare "journeyin/internal/share"
	"journeyin/internal/store"
)

func newUserManagementTestServer(t *testing.T, configureAuth bool) (*httptest.Server, *http.Client, *application.UserService, *Authenticator) {
	t.Helper()
	migrations, err := fs.Sub(journeyin.MigrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "users.db"), migrations)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	webFS, err := fs.Sub(journeyin.WebFS, "web/dist")
	if err != nil {
		t.Fatal(err)
	}
	schemaFS, err := fs.Sub(journeyin.SchemaFS, "schemas")
	if err != nil {
		t.Fatal(err)
	}
	app := application.NewTripService(database)
	users := application.NewUserService(database)
	if created, err := users.BootstrapAdminIfEmpty(context.Background(), "root", "bootstrap-password"); err != nil || !created {
		t.Fatalf("bootstrap admin: created=%v err=%v", created, err)
	}
	auth := NewAuthenticator("", "", "")
	if configureAuth {
		auth = NewAuthenticator("root", "bootstrap-password", "")
	}
	auth.SetUserService(users)
	api := NewServer(app, webFS, schemaFS, "test", nil)
	api.SetUserService(users)
	api.SetAuthenticator(auth)
	api.SetBundleService(journeyshare.NewBundleService(journeyshare.NewSQLiteBundleStore(database)))
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)
	return server, &http.Client{Jar: jar}, users, auth
}

func doUserJSON(t *testing.T, client *http.Client, method, url string, body any, token, ifMatch string) (*http.Response, []byte) {
	t.Helper()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, url, strings.NewReader(string(encoded)))
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return resp, data
}

func userTestTrip(title string) map[string]any {
	return map[string]any{
		"schema_version": 1, "title": title, "status": "draft", "timezone": "Asia/Shanghai",
		"date_range": map[string]any{"start": "2026-10-13", "end": "2026-10-13"},
		"days":       []any{map[string]any{"id": "day-1", "date": "2026-10-13", "stops": []any{map[string]any{"id": "stop-1", "sequence": 1, "title": "杭州"}}}},
	}
}

func TestNoAuthLocalModeAllowsDirectUseAndHonorsExplicitUserTokens(t *testing.T) {
	server, client, users, auth := newUserManagementTestServer(t, false)
	if auth.Enabled() {
		t.Fatal("database accounts alone should not force authentication")
	}
	me, body := doUserJSON(t, client, http.MethodGet, server.URL+"/api/v1/auth/me", nil, "", "")
	if me.StatusCode != http.StatusOK || !strings.Contains(string(body), "\"is_admin\":true") {
		t.Fatalf("local no-auth current user status=%d body=%s", me.StatusCode, body)
	}
	viewer, err := users.CreateUser(context.Background(), security.SystemPrincipal(), "local-viewer", "local-viewer-password", "user", "read_only", nil)
	if err != nil {
		t.Fatal(err)
	}
	token, err := users.CreateAPIToken(context.Background(), security.SystemPrincipal(), viewer.ID, "scoped client")
	if err != nil {
		t.Fatal(err)
	}
	tokenClient := &http.Client{}
	me, body = doUserJSON(t, tokenClient, http.MethodGet, server.URL+"/api/v1/auth/me", nil, token, "")
	if me.StatusCode != http.StatusOK || !strings.Contains(string(body), `"username":"local-viewer"`) || !strings.Contains(string(body), `"is_admin":false`) {
		t.Fatalf("scoped token current user status=%d body=%s", me.StatusCode, body)
	}
	invalid, body := doUserJSON(t, tokenClient, http.MethodGet, server.URL+"/api/v1/auth/me", nil, "invalid-token", "")
	if invalid.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid explicit bearer should not fall back to anonymous local mode: status=%d body=%s", invalid.StatusCode, body)
	}
}

func TestDatabaseUsersAdminAPIAndTripPermissions(t *testing.T) {
	server, adminClient, _, _ := newUserManagementTestServer(t, true)
	login, data := doUserJSON(t, adminClient, http.MethodPost, server.URL+"/api/v1/auth/login", map[string]string{"username": "root", "password": "bootstrap-password"}, "", "")
	if login.StatusCode != http.StatusOK {
		t.Fatalf("admin login status=%d body=%s", login.StatusCode, data)
	}
	current, data := doUserJSON(t, adminClient, http.MethodGet, server.URL+"/api/v1/auth/me", nil, "", "")
	if current.StatusCode != http.StatusOK || !strings.Contains(string(data), "\"is_admin\":true") {
		t.Fatalf("current user status=%d body=%s", current.StatusCode, data)
	}

	firstResp, firstData := doUserJSON(t, adminClient, http.MethodPost, server.URL+"/api/v1/trips", userTestTrip("Trip one"), "", "")
	secondResp, secondData := doUserJSON(t, adminClient, http.MethodPost, server.URL+"/api/v1/trips", userTestTrip("Trip two"), "", "")
	if firstResp.StatusCode != http.StatusCreated || secondResp.StatusCode != http.StatusCreated {
		t.Fatalf("create trip statuses=%d/%d bodies=%s/%s", firstResp.StatusCode, secondResp.StatusCode, firstData, secondData)
	}
	var first, second map[string]any
	if err := json.Unmarshal(firstData, &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(secondData, &second); err != nil {
		t.Fatal(err)
	}
	firstID := first["id"].(string)
	secondID := second["id"].(string)

	createResp, createData := doUserJSON(t, adminClient, http.MethodPost, server.URL+"/api/v1/admin/users", map[string]any{"username": "trip-editor", "password": "trip-editor-long-password", "role": "user", "access_mode": "selected_trip_edit", "editable_trip_ids": []string{firstID}}, "", "")
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create user status=%d body=%s", createResp.StatusCode, createData)
	}
	if strings.Contains(string(createData), "password_hash") {
		t.Fatalf("user response leaked password hash: %s", createData)
	}
	var user map[string]any
	if err := json.Unmarshal(createData, &user); err != nil {
		t.Fatal(err)
	}
	userID := user["id"].(string)

	tokenResp, tokenData := doUserJSON(t, adminClient, http.MethodPost, server.URL+"/api/v1/admin/users/"+userID+"/tokens", map[string]string{"name": "test MCP"}, "", "")
	if tokenResp.StatusCode != http.StatusCreated {
		t.Fatalf("issue API token status=%d body=%s", tokenResp.StatusCode, tokenData)
	}
	var tokenPayload map[string]any
	if err := json.Unmarshal(tokenData, &tokenPayload); err != nil {
		t.Fatal(err)
	}
	userToken, _ := tokenPayload["token"].(string)
	if userToken == "" {
		t.Fatalf("missing one-time token: %s", tokenData)
	}
	tokenList, tokenListData := doUserJSON(t, adminClient, http.MethodGet, server.URL+"/api/v1/admin/users/"+userID+"/tokens", nil, "", "")
	if tokenList.StatusCode != http.StatusOK || strings.Contains(string(tokenListData), userToken) || strings.Contains(string(tokenListData), "token_hash") {
		t.Fatalf("token metadata leaked a secret: status=%d body=%s", tokenList.StatusCode, tokenListData)
	}

	userClient := &http.Client{}
	for _, path := range []string{"/api/v1/admin/users", "/api/v1/settings", "/api/v1/photos/status"} {
		response, body := doUserJSON(t, userClient, http.MethodGet, server.URL+path, nil, userToken, "")
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("non-admin GET %s status=%d body=%s", path, response.StatusCode, body)
		}
	}
	adminCreate, adminBody := doUserJSON(t, userClient, http.MethodPost, server.URL+"/api/v1/admin/users", map[string]any{"username": "forbidden", "password": "user-cannot-create-this", "role": "admin", "access_mode": "all_trips_edit"}, userToken, "")
	if adminCreate.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin user management status=%d body=%s", adminCreate.StatusCode, adminBody)
	}
	tripList, listData := doUserJSON(t, userClient, http.MethodGet, server.URL+"/api/v1/trips", nil, userToken, "")
	if tripList.StatusCode != http.StatusOK {
		t.Fatalf("user trip list status=%d body=%s", tripList.StatusCode, listData)
	}
	var list struct {
		Items []struct {
			ID      string `json:"id"`
			CanEdit bool   `json:"can_edit"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listData, &list); err != nil {
		t.Fatal(err)
	}
	editability := map[string]bool{}
	for _, item := range list.Items {
		editability[item.ID] = item.CanEdit
	}
	if !editability[firstID] || editability[secondID] {
		t.Fatalf("wrong per-trip edit flags: %+v", editability)
	}

	writeBody := userTestTrip("edited")
	forbidden, forbiddenBody := doUserJSON(t, userClient, http.MethodPut, server.URL+"/api/v1/trips/"+secondID, writeBody, userToken, "revision-1")
	if forbidden.StatusCode != http.StatusForbidden {
		t.Fatalf("unassigned trip write status=%d body=%s", forbidden.StatusCode, forbiddenBody)
	}
	allowed, allowedBody := doUserJSON(t, userClient, http.MethodPut, server.URL+"/api/v1/trips/"+firstID, writeBody, userToken, "revision-1")
	if allowed.StatusCode != http.StatusOK {
		t.Fatalf("assigned trip write status=%d body=%s", allowed.StatusCode, allowedBody)
	}
	createForbidden, createBody := doUserJSON(t, userClient, http.MethodPost, server.URL+"/api/v1/trips", userTestTrip("not allowed"), userToken, "")
	if createForbidden.StatusCode != http.StatusForbidden {
		t.Fatalf("selected editor create status=%d body=%s", createForbidden.StatusCode, createBody)
	}
}

func TestSelectedEditorCanShareOnlyGrantedTrips(t *testing.T) {
	server, adminClient, _, _ := newUserManagementTestServer(t, true)
	defer server.Close()
	login, loginBody := doUserJSON(t, adminClient, http.MethodPost, server.URL+"/api/v1/auth/login", map[string]string{"username": "root", "password": "bootstrap-password"}, "", "")
	if login.StatusCode != http.StatusOK {
		t.Fatalf("admin login status=%d body=%s", login.StatusCode, loginBody)
	}
	firstResponse, firstBody := doUserJSON(t, adminClient, http.MethodPost, server.URL+"/api/v1/trips", userTestTrip("shareable"), "", "")
	secondResponse, secondBody := doUserJSON(t, adminClient, http.MethodPost, server.URL+"/api/v1/trips", userTestTrip("not assigned"), "", "")
	if firstResponse.StatusCode != http.StatusCreated || secondResponse.StatusCode != http.StatusCreated {
		t.Fatalf("trip create statuses=%d/%d bodies=%s/%s", firstResponse.StatusCode, secondResponse.StatusCode, firstBody, secondBody)
	}
	var first, second map[string]any
	if err := json.Unmarshal(firstBody, &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(secondBody, &second); err != nil {
		t.Fatal(err)
	}
	firstID, secondID := first["id"].(string), second["id"].(string)
	userResponse, userBody := doUserJSON(t, adminClient, http.MethodPost, server.URL+"/api/v1/admin/users", map[string]any{"username": "bundle-editor", "password": "bundle-editor-long-password", "role": "user", "access_mode": "selected_trip_edit", "editable_trip_ids": []string{firstID}}, "", "")
	if userResponse.StatusCode != http.StatusCreated {
		t.Fatalf("editor create status=%d body=%s", userResponse.StatusCode, userBody)
	}
	var user map[string]any
	if err := json.Unmarshal(userBody, &user); err != nil {
		t.Fatal(err)
	}
	tokenResponse, tokenBody := doUserJSON(t, adminClient, http.MethodPost, server.URL+"/api/v1/admin/users/"+user["id"].(string)+"/tokens", map[string]string{"name": "bundle test"}, "", "")
	if tokenResponse.StatusCode != http.StatusCreated {
		t.Fatalf("issue editor token status=%d body=%s", tokenResponse.StatusCode, tokenBody)
	}
	var tokenData map[string]any
	if err := json.Unmarshal(tokenBody, &tokenData); err != nil {
		t.Fatal(err)
	}
	userToken := tokenData["token"].(string)
	allowed, allowedBody := doUserJSON(t, &http.Client{}, http.MethodPost, server.URL+"/api/v1/share-bundles", map[string]any{"trip_ids": []string{firstID}, "ttl_seconds": 3600}, userToken, "")
	if allowed.StatusCode != http.StatusCreated {
		t.Fatalf("assigned editor share status=%d body=%s", allowed.StatusCode, allowedBody)
	}
	var share map[string]any
	if err := json.Unmarshal(allowedBody, &share); err != nil {
		t.Fatal(err)
	}
	listed, listBody := doUserJSON(t, &http.Client{}, http.MethodGet, server.URL+"/api/v1/share-bundles", nil, userToken, "")
	var ownShares struct {
		Items []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listBody, &ownShares); err != nil {
		t.Fatal(err)
	}
	if listed.StatusCode != http.StatusOK || len(ownShares.Items) != 1 || ownShares.Items[0].ID != share["id"].(string) || strings.Contains(string(listBody), userToken) {
		t.Fatalf("editor bundle list leaked or omitted own share: status=%d body=%s", listed.StatusCode, listBody)
	}
	forbidden, forbiddenBody := doUserJSON(t, &http.Client{}, http.MethodPost, server.URL+"/api/v1/share-bundles", map[string]any{"trip_ids": []string{firstID, secondID}, "ttl_seconds": 3600}, userToken, "")
	if forbidden.StatusCode != http.StatusForbidden {
		t.Fatalf("unassigned trip share status=%d body=%s", forbidden.StatusCode, forbiddenBody)
	}
}

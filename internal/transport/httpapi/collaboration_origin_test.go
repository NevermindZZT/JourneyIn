package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestCollaborationOriginMatchesPublicOrigin(t *testing.T) {
	server := &Server{publicURL: "https://collab.example:443/trip"}
	request := httptest.NewRequest("PUT", "http://internal/api/v1/collaboration/trips/trip-1", nil)
	request.Header.Set("Origin", "https://collab.example")
	if !server.collaborationOriginMatches(request) {
		t.Fatal("expected same HTTPS origin with implicit default port to match")
	}
	for _, origin := range []string{"https://evil.example", "https://collab.example.evil", "http://collab.example", "https://collab.example/other"} {
		request.Header.Set("Origin", origin)
		if server.collaborationOriginMatches(request) {
			t.Errorf("unexpected accepted collaboration Origin %q", origin)
		}
	}
}

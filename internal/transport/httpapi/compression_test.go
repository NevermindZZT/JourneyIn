package httpapi

import (
	"compress/gzip"
	"encoding/json"
	"net/http"
	"testing"
)

func TestAPIJSONResponsesUseNegotiatedGzip(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()
	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/capabilities", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept-Encoding", "gzip")
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Encoding") != "gzip" || response.Header.Get("Vary") != "Accept-Encoding" {
		t.Fatalf("unexpected gzip response: status=%d headers=%v", response.StatusCode, response.Header)
	}
	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var payload map[string]any
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["version"] == nil {
		t.Fatalf("gzip body did not contain capabilities JSON: %+v", payload)
	}
}

func TestAPIJSONResponsesRespectGzipQualityZero(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()
	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/capabilities", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept-Encoding", "gzip;q=0")
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("Content-Encoding") != "" {
		t.Fatalf("gzip must not be used when quality is zero: headers=%v", response.Header)
	}
}

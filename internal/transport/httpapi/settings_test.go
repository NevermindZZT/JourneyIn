package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestDefaultMapProviderSettingControlsCapabilitiesAndPlanning(t *testing.T) {
	server := testPlanningServer(t)
	defer server.Close()

	settingsResponse, err := http.Get(server.URL + "/api/v1/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer settingsResponse.Body.Close()
	var settings struct {
		Map struct {
			DefaultProvider string `json:"default_provider"`
		} `json:"map"`
	}
	if err := json.NewDecoder(settingsResponse.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.Map.DefaultProvider != "amap" {
		t.Fatalf("initial default provider=%q, want amap", settings.Map.DefaultProvider)
	}

	preferenceRequest, err := http.NewRequest(http.MethodPut, server.URL+"/api/v1/settings/map", strings.NewReader(`{"default_provider":"baidu"}`))
	if err != nil {
		t.Fatal(err)
	}
	preferenceRequest.Header.Set("Content-Type", "application/json")
	preferenceResponse, err := http.DefaultClient.Do(preferenceRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer preferenceResponse.Body.Close()
	if preferenceResponse.StatusCode != http.StatusOK {
		t.Fatalf("preference status=%d, want %d", preferenceResponse.StatusCode, http.StatusOK)
	}

	capabilitiesResponse, err := http.Get(server.URL + "/api/v1/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	defer capabilitiesResponse.Body.Close()
	var capabilities struct {
		DefaultProvider string `json:"default_map_provider"`
	}
	if err := json.NewDecoder(capabilitiesResponse.Body).Decode(&capabilities); err != nil {
		t.Fatal(err)
	}
	if capabilities.DefaultProvider != "baidu" {
		t.Fatalf("capabilities default provider=%q, want baidu", capabilities.DefaultProvider)
	}

	// Switch back to amap for the planning test
	revertRequest, err := http.NewRequest(http.MethodPut, server.URL+"/api/v1/settings/map", strings.NewReader(`{"default_provider":"amap"}`))
	if err != nil {
		t.Fatal(err)
	}
	revertRequest.Header.Set("Content-Type", "application/json")
	revertResponse, err := http.DefaultClient.Do(revertRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer revertResponse.Body.Close()
	if revertResponse.StatusCode != http.StatusOK {
		t.Fatalf("revert status=%d, want %d", revertResponse.StatusCode, http.StatusOK)
	}

	trip := `{"schema_version":1,"title":"default provider planning","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"map":{"enabled_providers":["amap"]},"days":[{"id":"day-1","date":"2026-04-18","stops":[{"id":"stop-a","sequence":1,"title":"A","location":{"preferred":"gcj02","coordinates":{"gcj02":{"lat":30.2,"lng":120.1,"crs":"gcj02"}}}},{"id":"stop-b","sequence":2,"title":"B","location":{"preferred":"gcj02","coordinates":{"gcj02":{"lat":30.21,"lng":120.11,"crs":"gcj02"}}}}]}]}`
	createResponse, err := http.Post(server.URL+"/api/v1/trips", "application/json", strings.NewReader(trip))
	if err != nil {
		t.Fatal(err)
	}
	defer createResponse.Body.Close()
	if createResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d, want %d", createResponse.StatusCode, http.StatusCreated)
	}
	var created struct {
		ID       string `json:"id"`
		Revision int    `json:"revision"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	planRequest, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/trips/"+created.ID+"/plan", strings.NewReader(`{"mode":"walking"}`))
	if err != nil {
		t.Fatal(err)
	}
	planRequest.Header.Set("Content-Type", "application/json")
	planRequest.Header.Set("If-Match", "revision-"+strconv.Itoa(created.Revision))
	planResponse, err := http.DefaultClient.Do(planRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer planResponse.Body.Close()
	if planResponse.StatusCode != http.StatusOK {
		t.Fatalf("plan status=%d, want %d", planResponse.StatusCode, http.StatusOK)
	}
	var planned struct {
		Document struct {
			Map struct {
				PreferredProvider string `json:"preferred_provider"`
				DefaultMode       string `json:"default_mode"`
			} `json:"map"`
			Days []struct {
				Legs []struct {
					Snapshots []struct {
						Provider string `json:"provider"`
					} `json:"snapshots"`
				} `json:"legs"`
			} `json:"days"`
		} `json:"document"`
	}
	if err := json.NewDecoder(planResponse.Body).Decode(&planned); err != nil {
		t.Fatal(err)
	}
	if planned.Document.Map.PreferredProvider != "amap" || planned.Document.Map.DefaultMode != "walking" || len(planned.Document.Days) != 1 || len(planned.Document.Days[0].Legs) != 1 || len(planned.Document.Days[0].Legs[0].Snapshots) != 1 || planned.Document.Days[0].Legs[0].Snapshots[0].Provider != "amap" {
		t.Fatalf("planned document did not persist AMap selection: %+v", planned.Document)
	}
	reloadResponse, err := http.Get(server.URL + "/api/v1/trips/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer reloadResponse.Body.Close()
	if reloadResponse.StatusCode != http.StatusOK {
		t.Fatalf("reload status=%d, want %d", reloadResponse.StatusCode, http.StatusOK)
	}
	if err := json.NewDecoder(reloadResponse.Body).Decode(&planned); err != nil {
		t.Fatal(err)
	}
	if planned.Document.Map.PreferredProvider != "amap" || planned.Document.Map.DefaultMode != "walking" || len(planned.Document.Days) != 1 || len(planned.Document.Days[0].Legs) != 1 || len(planned.Document.Days[0].Legs[0].Snapshots) != 1 || planned.Document.Days[0].Legs[0].Snapshots[0].Provider != "amap" {
		t.Fatalf("reloaded document lost AMap selection: %+v", planned.Document)
	}
}
func TestUpdatePhotosSettings(t *testing.T) {
	server := testPlanningServer(t)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/v1/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var s struct {
		Photos struct {
			RootDir    string `json:"root_dir"`
			Configured bool   `json:"configured"`
		} `json:"photos"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatal(err)
	}

	newDir := t.TempDir()
	bodyJSON, _ := json.Marshal(map[string]string{"root_dir": newDir})
	putReq, _ := http.NewRequest(http.MethodPut, server.URL+"/api/v1/settings/photos", strings.NewReader(string(bodyJSON)))
	putReq.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil || putResp.StatusCode != http.StatusOK {
		t.Fatalf("put photos setting failed: %v", err)
	}
	putResp.Body.Close()

	resp2, _ := http.Get(server.URL + "/api/v1/settings")
	var s2 struct {
		Photos struct {
			RootDir    string `json:"root_dir"`
			Configured bool   `json:"configured"`
		} `json:"photos"`
	}
	_ = json.NewDecoder(resp2.Body).Decode(&s2)
	resp2.Body.Close()
	if s2.Photos.RootDir != newDir || !s2.Photos.Configured {
		t.Fatalf("expected root_dir=%s, got %s", newDir, s2.Photos.RootDir)
	}

	clearReq, _ := http.NewRequest(http.MethodPut, server.URL+"/api/v1/settings/photos", strings.NewReader(`{"root_dir":""}`))
	clearReq.Header.Set("Content-Type", "application/json")
	clearResp, err := http.DefaultClient.Do(clearReq)
	if err != nil || clearResp.StatusCode != http.StatusOK {
		t.Fatalf("clear photos setting failed: %v", err)
	}
	clearResp.Body.Close()
}

func TestWeatherSettingsUpdateAndRead(t *testing.T) {
	server := testPlanningServer(t)
	defer server.Close()

	// 初始读取 weather 设置
	resp, err := http.Get(server.URL + "/api/v1/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var s struct {
		Weather struct {
			DefaultProvider string `json:"default_provider"`
			Caiyun          struct {
				TokenConfigured bool `json:"token_configured"`
			} `json:"caiyun"`
		} `json:"weather"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatal(err)
	}
	if s.Weather.DefaultProvider != "auto" {
		t.Fatalf("expected initial default weather provider auto, got %s", s.Weather.DefaultProvider)
	}
	if s.Weather.Caiyun.TokenConfigured {
		t.Fatalf("expected initial caiyun token not configured")
	}

	// 更新 weather 设置 (设置 default_provider 为 qweather，并填入 token/key/host)
	putReq, _ := http.NewRequest(http.MethodPut, server.URL+"/api/v1/settings/weather", strings.NewReader(`{"default_provider":"qweather","caiyun_token":"my_secret_token","qweather_key":"my_qw_key","qweather_host":"custom.xy.qweatherapi.com"}`))
	putReq.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil || putResp.StatusCode != http.StatusOK {
		t.Fatalf("put weather setting failed: %v", err)
	}
	putResp.Body.Close()

	// 再次读取检验状态
	resp2, _ := http.Get(server.URL + "/api/v1/settings")
	var s2 struct {
		Weather struct {
			DefaultProvider string `json:"default_provider"`
			OpenMeteo       struct {
				Available bool `json:"available"`
			} `json:"openmeteo"`
			QWeather struct {
				KeyConfigured bool   `json:"key_configured"`
				Host          string `json:"host"`
			} `json:"qweather"`
			Caiyun struct {
				TokenConfigured bool `json:"token_configured"`
			} `json:"caiyun"`
		} `json:"weather"`
	}
	_ = json.NewDecoder(resp2.Body).Decode(&s2)
	resp2.Body.Close()
	if s2.Weather.DefaultProvider != "qweather" {
		t.Fatalf("expected updated default weather provider qweather, got %s", s2.Weather.DefaultProvider)
	}
	if !s2.Weather.OpenMeteo.Available {
		t.Fatalf("expected openmeteo available to be true")
	}
	if !s2.Weather.QWeather.KeyConfigured || s2.Weather.QWeather.Host != "custom.xy.qweatherapi.com" {
		t.Fatalf("expected qweather key configured and custom host, got key=%v, host=%s", s2.Weather.QWeather.KeyConfigured, s2.Weather.QWeather.Host)
	}
	if !s2.Weather.Caiyun.TokenConfigured {
		t.Fatalf("expected caiyun token configured to be true")
	}

	// 清空彩云 Token
	clearReq, _ := http.NewRequest(http.MethodPut, server.URL+"/api/v1/settings/weather", strings.NewReader(`{"caiyun_token":""}`))
	clearReq.Header.Set("Content-Type", "application/json")
	clearResp, err := http.DefaultClient.Do(clearReq)
	if err != nil || clearResp.StatusCode != http.StatusOK {
		t.Fatalf("clear weather token failed: %v", err)
	}
	clearResp.Body.Close()

	resp3, _ := http.Get(server.URL + "/api/v1/settings")
	var s3 struct {
		Weather struct {
			Caiyun struct {
				TokenConfigured bool `json:"token_configured"`
			} `json:"caiyun"`
		} `json:"weather"`
	}
	_ = json.NewDecoder(resp3.Body).Decode(&s3)
	resp3.Body.Close()
	if s3.Weather.Caiyun.TokenConfigured {
		t.Fatalf("expected caiyun token to be cleared")
	}
}

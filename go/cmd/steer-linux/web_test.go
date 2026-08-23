// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	model "github.com/gsh20040816/openwrt-steer/go/internal/intent"
	linuxplatform "github.com/gsh20040816/openwrt-steer/go/internal/platform/linux"
)

func webTestIntent() model.Intent {
	return model.Intent{
		Main:        model.Main{ID: "main", SchemaVersion: model.SchemaVersion, Enabled: false, LogLevel: "warn", ProbeDirectURL: "https://direct.example/", ProbeProxyURL: "https://proxy.example/", SpeedtestProxyURL: "https://speed.example/"},
		Bootstrap:   model.Bootstrap{ID: "bootstrap", Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"},
		Routes:      []model.Route{{ID: "direct", Enabled: true, Kind: "direct"}, {ID: "block", Enabled: true, Kind: "block"}},
		DNSProfiles: []model.DNSProfile{{ID: "public", Enabled: true, Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"}},
		Rules:       []model.Rule{{ID: "default", Enabled: true, Default: true, DNSProfile: "public", Route: "direct"}},
	}
}

func TestWebConfigRequiresBearerAndIfMatch(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	tokenPath := filepath.Join(root, "web.token")
	if err := os.WriteFile(tokenPath, []byte("token-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := linuxplatform.JSONStore{Path: configPath}
	if _, err := store.Save(webTestIntent(), ""); err != nil {
		t.Fatal(err)
	}
	app := webApplication{TokenPath: tokenPath, ConfigPath: configPath, RunDirectory: filepath.Join(root, "run"), StateDirectory: filepath.Join(root, "state")}
	handler := app.auth(app.handleConfig)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	response := httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request returned %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	request.Header.Set("Authorization", "Bearer token-value")
	response = httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusOK || response.Header().Get("ETag") == "" {
		t.Fatalf("authenticated config request failed: status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	initialETag := response.Header().Get("ETag")
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || payload["intent"] == nil {
		t.Fatalf("config response is not canonical intent payload: %v %#v", err, payload)
	}
	encodedIntent, err := json.Marshal(map[string]any{"intent": webTestIntent()})
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader(string(encodedIntent)))
	request.Header.Set("Authorization", "Bearer token-value")
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing If-Match returned %d: %s", response.Code, response.Body.String())
	}
	updated := webTestIntent()
	updated.Main.LogLevel = "info"
	encodedIntent, err = json.Marshal(map[string]any{"intent": updated})
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader(string(encodedIntent)))
	request.Header.Set("Authorization", "Bearer token-value")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", initialETag)
	response = httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusOK || response.Header().Get("ETag") == "" {
		t.Fatalf("first config save failed to return a new ETag: status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	newETag := response.Header().Get("ETag")
	request = httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader(string(encodedIntent)))
	request.Header.Set("Authorization", "Bearer token-value")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", newETag)
	response = httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("second config save rejected the returned ETag: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestWebAssetsRunUnderStrictCSP(t *testing.T) {
	root := t.TempDir()
	app := webApplication{TokenPath: filepath.Join(root, "web.token"), ConfigPath: filepath.Join(root, "config.json"), RunDirectory: filepath.Join(root, "run"), StateDirectory: filepath.Join(root, "state")}
	handler := webHandler(app)

	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK || page.Header().Get("Content-Security-Policy") != "default-src 'self'; style-src 'self'; script-src 'self'" {
		t.Fatalf("strict CSP was not returned: status=%d headers=%v", page.Code, page.Header())
	}
	if strings.Contains(page.Body.String(), "const token =") || !strings.Contains(page.Body.String(), `src="/app.js"`) {
		t.Fatalf("index still contains inline application code: %s", page.Body.String())
	}

	for path, contentType := range map[string]string{"/app.js": "application/javascript; charset=utf-8", "/style.css": "text/css; charset=utf-8"} {
		asset := httptest.NewRecorder()
		handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, path, nil))
		if asset.Code != http.StatusOK || asset.Header().Get("Content-Type") != contentType || asset.Body.Len() == 0 {
			t.Fatalf("asset %s failed: status=%d type=%q body=%d", path, asset.Code, asset.Header().Get("Content-Type"), asset.Body.Len())
		}
	}
}

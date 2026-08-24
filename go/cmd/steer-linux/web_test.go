// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsh20040816/steer/go/internal/geodata"
	model "github.com/gsh20040816/steer/go/internal/intent"
	linuxplatform "github.com/gsh20040816/steer/go/internal/platform/linux"
)

type webRuntimeRunner struct{}

func (webRuntimeRunner) Output(_ context.Context, name string, _ ...string) ([]byte, error) {
	switch name {
	case "/usr/bin/sing-box":
		return []byte("sing-box version 1.14.0-rc.1\nTags: with_quic,with_utls\n"), nil
	default:
		return nil, fmt.Errorf("unexpected runtime command %s", name)
	}
}

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
	webConfigPath := filepath.Join(root, "web.json")
	const token = "test-token-value-0123456789-abcdef"
	if err := os.WriteFile(webConfigPath, []byte(`{"schema_version":1,"token":"`+token+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := linuxplatform.IntentStore{Path: configPath}
	if _, err := store.Save(webTestIntent(), ""); err != nil {
		t.Fatal(err)
	}
	app := webApplication{WebConfigPath: webConfigPath, ConfigPath: configPath, RunDirectory: filepath.Join(root, "run"), StateDirectory: filepath.Join(root, "state")}
	handler := app.auth(app.handleConfig)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	response := httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request returned %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	request.Header.Set("Authorization", "Bearer "+token)
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
	request.Header.Set("Authorization", "Bearer "+token)
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
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", initialETag)
	response = httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusOK || response.Header().Get("ETag") == "" {
		t.Fatalf("first config save failed to return a new ETag: status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	newETag := response.Header().Get("ETag")
	request = httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader(string(encodedIntent)))
	request.Header.Set("Authorization", "Bearer "+token)
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
	app := webApplication{WebConfigPath: filepath.Join(root, "web.json"), ConfigPath: filepath.Join(root, "config.json"), RunDirectory: filepath.Join(root, "run"), StateDirectory: filepath.Join(root, "state")}
	handler := webHandler(app)

	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK || page.Header().Get("Content-Security-Policy") != "default-src 'self'; style-src 'self'; script-src 'self'" {
		t.Fatalf("strict CSP was not returned: status=%d headers=%v", page.Code, page.Header())
	}
	if strings.Contains(page.Body.String(), "const token =") || !strings.Contains(page.Body.String(), `src="/app.js"`) {
		t.Fatalf("index still contains inline application code: %s", page.Body.String())
	}
	if !strings.Contains(page.Body.String(), `id="side"`) || !strings.Contains(page.Body.String(), `src="/js/views/system.js"`) {
		t.Fatalf("index has no redesigned shell or system view: %s", page.Body.String())
	}

	for path, contentType := range map[string]string{
		"/app.js":             "application/javascript; charset=utf-8",
		"/style.css":          "text/css; charset=utf-8",
		"/js/api.js":          "application/javascript; charset=utf-8",
		"/js/views/system.js": "application/javascript; charset=utf-8",
	} {
		asset := httptest.NewRecorder()
		handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, path, nil))
		if asset.Code != http.StatusOK || asset.Header().Get("Content-Type") != contentType || asset.Body.Len() == 0 {
			t.Fatalf("asset %s failed: status=%d type=%q body=%d", path, asset.Code, asset.Header().Get("Content-Type"), asset.Body.Len())
		}
	}
	apiScript := httptest.NewRecorder()
	handler.ServeHTTP(apiScript, httptest.NewRequest(http.MethodGet, "/js/api.js", nil))
	if strings.Contains(apiScript.Body.String(), "/api/v1/platform") || !strings.Contains(apiScript.Body.String(), "validateGeoCategories") || strings.Contains(apiScript.Body.String(), "category.split('@', 1)[0]") || strings.Contains(apiScript.Body.String(), "sha256") {
		t.Fatalf("API adapter retained platform settings, lost exact Geo validation or exposes hashes: %s", apiScript.Body.String())
	}
	systemScript := httptest.NewRecorder()
	handler.ServeHTTP(systemScript, httptest.NewRequest(http.MethodGet, "/js/views/system.js", nil))
	if strings.Contains(systemScript.Body.String(), "0.5.0") || strings.Contains(systemScript.Body.String(), "0.9.0") || !strings.Contains(systemScript.Body.String(), "runtime") {
		t.Fatalf("system view still contains stale version facts: %s", systemScript.Body.String())
	}
}

func TestWebRuntimeReportsInstalledToolVersions(t *testing.T) {
	root := t.TempDir()
	seedDirectory := writeWebSeed(t, root)
	app := webApplication{Runner: webRuntimeRunner{}, SeedDirectory: seedDirectory}
	response := httptest.NewRecorder()
	app.handleRuntime(response, httptest.NewRequest(http.MethodGet, "/api/v1/runtime", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("runtime endpoint returned %d: %s", response.Code, response.Body.String())
	}
	var value runtimeInfo
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatalf("runtime response is not JSON: %v", err)
	}
	if value.Steer != version || value.SingBox.Version != "1.14.0-rc.1" || value.GeoData.Version != "test" || value.GeoData.RuleCount != 1 || value.CanonicalSchema != model.SchemaVersion {
		t.Fatalf("runtime response = %#v", value)
	}
	if value.SingBox.Error != "" || value.GeoData.Error != "" || strings.Join(value.SingBox.Tags, ",") != "with_quic,with_utls" {
		t.Fatalf("runtime dependency details = %#v", value)
	}
}

func TestWebTokenPrintsConfiguredCredential(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "web.json")
	const token = "user-configured-token-0123456789-abcd"
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"token":"`+token+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = write
	err = runWebToken([]string{"-config", path})
	write.Close()
	os.Stdout = original
	output, readErr := io.ReadAll(read)
	read.Close()
	if err != nil || readErr != nil {
		t.Fatalf("web-token failed: command=%v read=%v", err, readErr)
	}
	if string(output) != token+"\n" {
		t.Fatalf("web-token output = %q", output)
	}
}

func TestWebGeoDataReturnsStatusResource(t *testing.T) {
	root := t.TempDir()
	seedDirectory := writeWebSeed(t, root)
	app := webApplication{SeedDirectory: seedDirectory}
	response := httptest.NewRecorder()
	app.handleGeoData(response, httptest.NewRequest(http.MethodGet, "/api/v1/geodata/geosite", nil))
	var ready geoDataStatus
	if err := json.Unmarshal(response.Body.Bytes(), &ready); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !ready.Readable || ready.Count != 1 || len(ready.Names) != 1 || ready.Names[0] != "cn@ads" || ready.Error != nil {
		t.Fatalf("ready Geo status = %#v status=%d", ready, response.Code)
	}
}

func writeWebSeed(t *testing.T, root string) string {
	t.Helper()
	seedDirectory := filepath.Join(root, "seed")
	rulesDirectory := filepath.Join(seedDirectory, "rules")
	if err := os.MkdirAll(rulesDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	const tag = "steer-geosite-cn@ads"
	content := []byte("compiled\n")
	if err := os.WriteFile(filepath.Join(rulesDirectory, tag+".srs"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	ruleSum := sha256.Sum256(content)
	inputSum := sha256.Sum256([]byte("input"))
	manifest := geodata.Manifest{
		SchemaVersion: geodata.ManifestSchemaVersion,
		Upstream: geodata.UpstreamIdentity{
			Repository: geodata.UpstreamRepository, Version: "test",
			GeoSiteSHA256: hex.EncodeToString(inputSum[:]), GeoIPSHA256: hex.EncodeToString(inputSum[:]),
		},
		Tools: geodata.ToolIdentity{GeoViewRef: geodata.GeoViewCommit, SingBoxVersion: geodata.SingBoxCompiler},
		Rules: []geodata.Rule{{
			Kind: "geosite", Category: "cn@ads", Tag: tag, Path: "rules/" + tag + ".srs",
			SHA256: hex.EncodeToString(ruleSum[:]), Size: int64(len(content)),
		}},
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seedDirectory, "manifest.json"), encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	return seedDirectory
}

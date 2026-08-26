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
	"time"

	coreapply "github.com/gsh20040816/steer/go/internal/apply"
	"github.com/gsh20040816/steer/go/internal/geodata"
	model "github.com/gsh20040816/steer/go/internal/intent"
	linuxplatform "github.com/gsh20040816/steer/go/internal/platform/linux"
	"github.com/gsh20040816/steer/go/internal/probe"
)

type webRuntimeRunner struct{}

type webLogRunner struct{ arguments []string }

func (runner *webLogRunner) Output(_ context.Context, name string, arguments ...string) ([]byte, error) {
	if name != "/usr/bin/journalctl" {
		return nil, fmt.Errorf("unexpected log command %s", name)
	}
	runner.arguments = append([]string{}, arguments...)
	return []byte("combined Steer logs"), nil
}

func (webRuntimeRunner) Output(_ context.Context, name string, _ ...string) ([]byte, error) {
	switch name {
	case "/usr/bin/sing-box":
		return []byte("sing-box version 1.14.0-rc.1\nTags: with_quic,with_utls\n"), nil
	default:
		return nil, fmt.Errorf("unexpected runtime command %s", name)
	}
}

type webApplyRunner struct{ failStop bool }

func (runner webApplyRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	if name == "/usr/bin/systemctl" && len(args) > 0 && args[0] == "stop" {
		if runner.failStop {
			return nil, fmt.Errorf("systemd refused stop")
		}
		return []byte{}, nil
	}
	if name == "/usr/sbin/nft" && strings.Join(args, " ") == "-j list tables" {
		return []byte(`{"nftables":[]}`), nil
	}
	return nil, fmt.Errorf("unexpected Apply command %s %s", name, strings.Join(args, " "))
}

func webTestIntent() model.Intent {
	return model.Intent{
		Main:        model.Main{ID: "main", SchemaVersion: model.SchemaVersion, Enabled: false, LogLevel: "warn", ProbeDirectURL: "https://direct.example/", ProbeProxyURL: "https://proxy.example/", SpeedtestProxyURL: "https://speed.example/"},
		Bootstrap:   model.Bootstrap{ID: "bootstrap", Protocol: "udp", Server: "1.1.1.1", ServerPort: 53, Strategy: "prefer_ipv4"},
		Routes:      []model.Route{{ID: "direct", Enabled: true, Kind: "direct"}, {ID: "block", Enabled: true, Kind: "block"}},
		DNSProfiles: []model.DNSProfile{{ID: "public", Enabled: true, Protocol: "udp", Server: "1.1.1.1", ServerPort: 53}},
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

func TestWebApplySavedPersistsFailureAndClearsPendingOnSuccess(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	runDirectory := filepath.Join(root, "run")
	store := linuxplatform.IntentStore{Path: configPath}
	if _, err := store.Save(webTestIntent(), ""); err != nil {
		t.Fatal(err)
	}

	failedApp := webApplication{
		ConfigPath: configPath, RunDirectory: runDirectory, StateDirectory: filepath.Join(root, "state"),
		SeedDirectory: filepath.Join(root, "seed"), Runner: webApplyRunner{failStop: true},
	}
	failedResponse := httptest.NewRecorder()
	failedApp.handleApply(failedResponse, httptest.NewRequest(http.MethodPost, "/api/v1/apply", nil))
	var failed coreapply.Result
	if err := json.Unmarshal(failedResponse.Body.Bytes(), &failed); err != nil {
		t.Fatal(err)
	}
	if failedResponse.Code != http.StatusUnprocessableEntity || failed.OK || failed.Error == "" || failed.RuntimeDigest == "" || failed.Generation != "" || failed.Activated {
		t.Fatalf("failed Apply response = %#v status=%d body=%s", failed, failedResponse.Code, failedResponse.Body.String())
	}
	recordContent, err := os.ReadFile(filepath.Join(runDirectory, "last-apply.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record coreapply.Record
	if err := json.Unmarshal(recordContent, &record); err != nil {
		t.Fatal(err)
	}
	if record.Timestamp == "" || record.Sequence == "" || record.Result.Error != failed.Error {
		t.Fatalf("persistent Apply record = %#v", record)
	}

	overviewResponse := httptest.NewRecorder()
	failedApp.handleOverview(overviewResponse, httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil))
	var failedOverview struct {
		PendingApply bool                 `json:"pending_apply"`
		Status       linuxplatform.Status `json:"status"`
	}
	if err := json.Unmarshal(overviewResponse.Body.Bytes(), &failedOverview); err != nil {
		t.Fatal(err)
	}
	if !failedOverview.PendingApply || failedOverview.Status.LastApply == nil || failedOverview.Status.LastApply.Result.Error == "" {
		t.Fatalf("failed Apply was not persistent/pending: %#v body=%s", failedOverview, overviewResponse.Body.String())
	}

	successApp := failedApp
	successApp.Runner = webApplyRunner{}
	successResponse := httptest.NewRecorder()
	successApp.handleApply(successResponse, httptest.NewRequest(http.MethodPost, "/api/v1/apply", nil))
	var succeeded coreapply.Result
	if err := json.Unmarshal(successResponse.Body.Bytes(), &succeeded); err != nil {
		t.Fatal(err)
	}
	if successResponse.Code != http.StatusOK || !succeeded.OK {
		t.Fatalf("successful Apply response = %#v status=%d body=%s", succeeded, successResponse.Code, successResponse.Body.String())
	}
	overviewResponse = httptest.NewRecorder()
	successApp.handleOverview(overviewResponse, httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil))
	var successfulOverview struct {
		PendingApply bool `json:"pending_apply"`
	}
	if err := json.Unmarshal(overviewResponse.Body.Bytes(), &successfulOverview); err != nil {
		t.Fatal(err)
	}
	if successfulOverview.PendingApply {
		t.Fatalf("successful Apply did not clear pending: %s", overviewResponse.Body.String())
	}
}

func TestWebOverviewMarksEnabledSavedConfigPendingWithoutCurrent(t *testing.T) {
	root := t.TempDir()
	value := webTestIntent()
	value.Main.Enabled = true
	configPath := filepath.Join(root, "config.json")
	if _, err := (linuxplatform.IntentStore{Path: configPath}).Save(value, ""); err != nil {
		t.Fatal(err)
	}
	app := webApplication{ConfigPath: configPath, RunDirectory: filepath.Join(root, "run"), StateDirectory: filepath.Join(root, "state"), SeedDirectory: filepath.Join(root, "seed")}
	response := httptest.NewRecorder()
	app.handleOverview(response, httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil))
	var overview struct {
		PendingApply bool                 `json:"pending_apply"`
		SavedEnabled bool                 `json:"saved_enabled"`
		Status       linuxplatform.Status `json:"status"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !overview.PendingApply || !overview.SavedEnabled || overview.Status.Generation != "" {
		t.Fatalf("enabled saved config pending overview = %#v body=%s", overview, response.Body.String())
	}
}

func TestWebConfigRejectsUnknownGeoSelectorWithoutSaving(t *testing.T) {
	root := t.TempDir()
	seedDirectory := writeWebSeed(t, root)
	configPath := filepath.Join(root, "config.json")
	store := linuxplatform.IntentStore{Path: configPath, GeoDataDirectory: seedDirectory}
	base := webTestIntent()
	revision, err := store.Save(base, "")
	if err != nil {
		t.Fatal(err)
	}
	candidate := webTestIntent()
	candidate.Rules = append([]model.Rule{{
		ID: "unknown", Enabled: true, DNSProfile: "public", Route: "direct",
		DomainMatch: []string{"geosite:not-installed"},
	}}, candidate.Rules...)
	body, err := json.Marshal(map[string]any{"intent": candidate})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader(string(body)))
	request.Header.Set("If-Match", revision)
	response := httptest.NewRecorder()
	app := webApplication{ConfigPath: configPath, RunDirectory: filepath.Join(root, "run"), SeedDirectory: seedDirectory}
	app.handleConfig(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown selector returned %d: %s", response.Code, response.Body.String())
	}
	var result struct {
		Saved      bool             `json:"saved"`
		Validation model.Validation `json:"validation"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Saved || result.Validation.OK {
		t.Fatalf("invalid candidate response = %#v", result)
	}
	found := false
	for _, issue := range result.Validation.Errors {
		if issue.Code == geodata.ErrorCategoryNotFound && issue.ObjectID == "unknown" && issue.Option == "domain_match" {
			found = true
		}
	}
	if !found {
		t.Fatalf("structured Geo issue is missing: %#v", result.Validation.Errors)
	}
	loaded, loadedRevision, err := store.Load()
	if err != nil || loadedRevision != revision || len(loaded.Rules) != len(base.Rules) {
		t.Fatalf("invalid candidate changed the saved config: revision=%q value=%#v err=%v", loadedRevision, loaded, err)
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
	if strings.Contains(apiScript.Body.String(), "/api/v1/platform") || strings.Contains(apiScript.Body.String(), "validateGeoCategories") || strings.Contains(apiScript.Body.String(), "category.split('@', 1)[0]") || strings.Contains(apiScript.Body.String(), "sha256") {
		t.Fatalf("API adapter retained platform settings, duplicates canonical Geo validation or exposes hashes: %s", apiScript.Body.String())
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

func TestWebDiagnosticsReturnsSanitizedHistoryAndAggregatedLogs(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	stateDirectory := filepath.Join(root, "state")
	if _, err := (linuxplatform.IntentStore{Path: configPath}).Save(webTestIntent(), ""); err != nil {
		t.Fatal(err)
	}
	if err := probe.SaveReport(stateDirectory, probe.Report{
		Scope: "nodes", ObjectID: "node_a", Kind: "connect", TestedAt: time.Now(),
		Error: "temporary sing-box password=secret", Results: []probe.Result{{URL: "https://user:token@example.test/probe?token=secret"}},
	}); err != nil {
		t.Fatal(err)
	}
	runner := &webLogRunner{}
	app := webApplication{ConfigPath: configPath, RunDirectory: filepath.Join(root, "run"), StateDirectory: stateDirectory, Runner: runner}
	diagnosticsResponse := httptest.NewRecorder()
	app.handleDiagnostics(diagnosticsResponse, httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics", nil))
	if diagnosticsResponse.Code != http.StatusOK {
		t.Fatalf("diagnostics endpoint returned %d: %s", diagnosticsResponse.Code, diagnosticsResponse.Body.String())
	}
	var diagnostics probe.Diagnostics
	if err := json.Unmarshal(diagnosticsResponse.Body.Bytes(), &diagnostics); err != nil || len(diagnostics.Reports) != 1 || diagnostics.SavedDigest == "" {
		t.Fatalf("diagnostics response drifted: %v %#v", err, diagnostics)
	}
	encoded := diagnosticsResponse.Body.String()
	for _, secret := range []string{"user:token", "secret", "temporary sing-box"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("diagnostics response leaked %q: %s", secret, encoded)
		}
	}

	logsResponse := httptest.NewRecorder()
	app.handleLogs(logsResponse, httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil))
	arguments := strings.Join(runner.arguments, " ")
	for _, unit := range []string{"steer.service", "steer-web.service", "steer-subscription.service"} {
		if !strings.Contains(arguments, unit) {
			t.Fatalf("aggregated journal omitted %s: %s", unit, arguments)
		}
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

func TestWebNodeImportUsesSharedMultiNodeParser(t *testing.T) {
	document := "vless://00000000-0000-4000-8000-000000000001@example.com:443?security=tls&sni=edge.example.com&type=ws&path=%2Fproxy\n" +
		"not-a-node\n" +
		"socks5://user:password@example.com:1080#SOCKS"
	body, err := json.Marshal(map[string]string{"document": document})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	webApplication{}.handleNodeImport(response, httptest.NewRequest(http.MethodPost, "/api/v1/nodes/import", strings.NewReader(string(body))))
	var result struct {
		Nodes   []model.Node `json:"nodes"`
		Skipped int          `json:"skipped"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(result.Nodes) != 2 || result.Skipped != 1 || result.Nodes[0].Type != "vless" || result.Nodes[1].Type != "socks" {
		t.Fatalf("multi-node import = %#v status=%d body=%s", result, response.Code, response.Body.String())
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

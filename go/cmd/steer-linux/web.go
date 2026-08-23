// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	coreapply "github.com/gsh20040816/openwrt-steer/go/internal/apply"
	"github.com/gsh20040816/openwrt-steer/go/internal/geodata"
	model "github.com/gsh20040816/openwrt-steer/go/internal/intent"
	linuxplatform "github.com/gsh20040816/openwrt-steer/go/internal/platform/linux"
	"github.com/gsh20040816/openwrt-steer/go/internal/subscription"
)

//go:embed web/*
var webAssets embed.FS

type webApplication struct {
	TokenPath      string
	ConfigPath     string
	RunDirectory   string
	StateDirectory string
}

func serveWeb(listen, tokenPath, configPath, runDirectory, stateDirectory string) error {
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		return fmt.Errorf("read Web token (run web-token first): %w", err)
	}
	token = bytes.TrimSpace(token)
	if len(token) < 32 {
		return errors.New("Web token is too short")
	}
	app := webApplication{TokenPath: tokenPath, ConfigPath: configPath, RunDirectory: runDirectory, StateDirectory: stateDirectory}
	return (&http.Server{Addr: listen, Handler: webHandler(app)}).ListenAndServe()
}

func webHandler(app webApplication) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", app.handleIndex)
	mux.HandleFunc("/app.js", app.handleAsset)
	mux.HandleFunc("/style.css", app.handleAsset)
	mux.HandleFunc("/api/v1/config", app.auth(app.handleConfig))
	mux.HandleFunc("/api/v1/overview", app.auth(app.handleOverview))
	mux.HandleFunc("/api/v1/validate", app.auth(app.handleValidate))
	mux.HandleFunc("/api/v1/apply", app.auth(app.handleApply))
	mux.HandleFunc("/api/v1/nodes/import", app.auth(app.handleNodeImport))
	mux.HandleFunc("/api/v1/probes/", app.auth(app.handleProbes))
	mux.HandleFunc("/api/v1/subscriptions", app.auth(app.handleSubscriptions))
	mux.HandleFunc("/api/v1/subscriptions/", app.auth(app.handleSubscriptions))
	mux.HandleFunc("/api/v1/geodata/", app.auth(app.handleGeoData))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(writer, request)
	})
}

func (app webApplication) handleIndex(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		http.NotFound(writer, request)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	content, err := webAssets.ReadFile("web/index.html")
	if err != nil {
		http.Error(writer, "web asset is unavailable", http.StatusInternalServerError)
		return
	}
	_, _ = writer.Write(content)
}

func (app webApplication) handleAsset(writer http.ResponseWriter, request *http.Request) {
	assets := map[string]string{"/app.js": "web/app.js", "/style.css": "web/style.css"}
	asset, ok := assets[request.URL.Path]
	if !ok || (request.Method != http.MethodGet && request.Method != http.MethodHead) {
		if !ok {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Allow", "GET, HEAD")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	content, err := webAssets.ReadFile(asset)
	if err != nil {
		http.Error(writer, "web asset is unavailable", http.StatusInternalServerError)
		return
	}
	if strings.HasSuffix(asset, ".js") {
		writer.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	} else {
		writer.Header().Set("Content-Type", "text/css; charset=utf-8")
	}
	if request.Method == http.MethodGet {
		_, _ = writer.Write(content)
	}
}

func (app webApplication) auth(handler http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if !app.authorized(request) {
			writer.Header().Set("WWW-Authenticate", `Bearer realm="steer"`)
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		if request.Method != http.MethodGet && request.Method != http.MethodHead && !sameOrigin(request) {
			http.Error(writer, "origin is not allowed", http.StatusForbidden)
			return
		}
		handler(writer, request)
	}
}

func (app webApplication) authorized(request *http.Request) bool {
	stored, err := os.ReadFile(app.TokenPath)
	if err != nil {
		return false
	}
	stored = bytes.TrimSpace(stored)
	header := request.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	value := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if value == "" || len(value) != len(stored) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(value), stored) == 1
}

func sameOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	return origin == "" || strings.HasSuffix(origin, "://"+request.Host)
}

func (app webApplication) handleConfig(writer http.ResponseWriter, request *http.Request) {
	store := linuxplatform.JSONStore{Path: app.ConfigPath}
	switch request.Method {
	case http.MethodGet:
		value, revision, err := store.Load()
		if err != nil {
			writeWebError(writer, err, http.StatusInternalServerError)
			return
		}
		writer.Header().Set("ETag", revision)
		writeWebJSON(writer, map[string]any{"intent": value, "revision": revision})
	case http.MethodPut:
		body, err := io.ReadAll(io.LimitReader(request.Body, 2<<20))
		if err != nil {
			writeWebError(writer, err, http.StatusBadRequest)
			return
		}
		value, apply, err := decodeIntentPayload(body)
		if err != nil {
			writeWebError(writer, err, http.StatusBadRequest)
			return
		}
		expectedRevision := request.Header.Get("If-Match")
		var revision string
		var saveErr error
		var applyResult coreapply.Result
		var applyErr error
		lockErr := withOperationLock(app.RunDirectory, func() error {
			if expectedRevision == "" {
				if _, statErr := os.Stat(store.Path); statErr == nil {
					saveErr = errors.New("If-Match is required for an existing configuration")
					return nil
				}
			}
			revision, saveErr = store.Save(value, expectedRevision)
			if saveErr != nil || !apply {
				return nil
			}
			// Save and Apply use the exact value submitted by this request while
			// holding the same operation lock. A subscription timer cannot
			// replace it between the two steps.
			applyResult, applyErr = app.applyValue(value)
			applyResult, applyErr = recordApplyResult(app.RunDirectory, applyResult, applyErr)
			return nil
		})
		if lockErr != nil {
			writeWebError(writer, lockErr, http.StatusInternalServerError)
			return
		}
		if errors.Is(saveErr, linuxplatform.ErrRevisionConflict) {
			writeWebError(writer, saveErr, http.StatusConflict)
			return
		}
		if saveErr != nil {
			status := http.StatusUnprocessableEntity
			if strings.Contains(saveErr.Error(), "If-Match is required") {
				status = http.StatusPreconditionRequired
			}
			writeWebError(writer, saveErr, status)
			return
		}
		writer.Header().Set("ETag", revision)
		response := map[string]any{"saved": true, "applied": false, "revision": revision}
		if apply {
			response["apply_result"] = applyResult
			if applyErr == nil {
				response["applied"] = true
			}
		}
		if applyErr != nil {
			writeWebJSONStatus(writer, response, http.StatusUnprocessableEntity)
			return
		}
		writeWebJSON(writer, response)
	default:
		writer.Header().Set("Allow", "GET, PUT")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (app webApplication) handleOverview(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store := linuxplatform.JSONStore{Path: app.ConfigPath}
	value, revision, loadErr := store.Load()
	validation := model.Validation{OK: false}
	if loadErr == nil {
		validation = linuxplatform.Validate(value)
	}
	status := linuxplatform.ReadStatus(request.Context(), linuxplatform.ExecRunner{}, linuxplatform.BackendOptions{RunDirectory: app.RunDirectory})
	writeWebJSON(writer, map[string]any{"saved_revision": revision, "saved_valid": loadErr == nil && validation.OK, "validation": validation, "status": status, "error": errorString(loadErr)})
}

func (app webApplication) handleValidate(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, 2<<20))
	if err != nil {
		writeWebError(writer, err, http.StatusBadRequest)
		return
	}
	value, _, err := decodeIntentPayload(body)
	if err != nil {
		writeWebError(writer, err, http.StatusBadRequest)
		return
	}
	validation := linuxplatform.Validate(value)
	writeWebJSON(writer, validation)
	if !validation.OK {
		return
	}
}

func (app webApplication) handleApply(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := app.applySaved()
	if err != nil {
		writeWebJSONStatus(writer, result, http.StatusUnprocessableEntity)
		return
	}
	writeWebJSON(writer, result)
}

func (app webApplication) handleNodeImport(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		URI string `json:"uri"`
	}
	if err := json.NewDecoder(io.LimitReader(request.Body, 256<<10)).Decode(&payload); err != nil || payload.URI == "" {
		writeWebError(writer, errors.New("request requires a uri"), http.StatusBadRequest)
		return
	}
	node, err := subscription.ParseURI(payload.URI)
	if err == nil {
		validation := model.ValidateNode(node)
		if !validation.OK {
			err = errors.New("imported node failed validation")
		}
	}
	if err != nil {
		writeWebError(writer, err, http.StatusUnprocessableEntity)
		return
	}
	writeWebJSON(writer, map[string]any{"node": node})
}

func (app webApplication) handleProbes(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(request.URL.Path, "/api/v1/probes/")
	body, _ := io.ReadAll(io.LimitReader(request.Body, 64<<10))
	var payload struct {
		Kind     string `json:"kind"`
		Download bool   `json:"download"`
	}
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			writeWebError(writer, err, http.StatusBadRequest)
			return
		}
	}
	if path == "overview" {
		kind := payload.Kind
		if kind == "" {
			kind = "direct"
		}
		report, err := linuxplatform.ProbeCurrentWithState(request.Context(), app.RunDirectory, app.StateDirectory, kind, nil)
		if err != nil {
			writeWebError(writer, err, http.StatusUnprocessableEntity)
			return
		}
		writeWebJSON(writer, report)
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 2 && (parts[0] == "nodes" || parts[0] == "routes") {
		var report linuxplatform.TestReport
		var err error
		if parts[0] == "nodes" {
			report, err = linuxplatform.SpeedTestNode(request.Context(), app.ConfigPath, app.StateDirectory, "/usr/bin/sing-box", parts[1], payload.Download)
		} else {
			report, err = linuxplatform.SpeedTestRoute(request.Context(), app.ConfigPath, app.StateDirectory, "/usr/bin/sing-box", parts[1], payload.Download)
		}
		if err != nil {
			writeWebError(writer, err, http.StatusUnprocessableEntity)
			return
		}
		writeWebJSON(writer, report)
		return
	}
	http.NotFound(writer, request)
}

func (app webApplication) handleSubscriptions(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodGet && request.URL.Path == "/api/v1/subscriptions" {
		statuses, err := linuxplatform.ReadSubscriptionStatus(app.ConfigPath, app.StateDirectory)
		if err != nil {
			writeWebError(writer, err, http.StatusInternalServerError)
			return
		}
		writeWebJSON(writer, map[string]any{"subscriptions": statuses})
		return
	}
	prefix := "/api/v1/subscriptions/"
	if !strings.HasPrefix(request.URL.Path, prefix) {
		http.NotFound(writer, request)
		return
	}
	remainder := strings.TrimPrefix(request.URL.Path, prefix)
	parts := strings.Split(strings.TrimSuffix(remainder, "/"), "/")
	if len(parts) == 2 && parts[1] == "update" && request.Method == http.MethodPost {
		var snapshots []linuxplatform.SubscriptionSnapshot
		err := withOperationLock(app.RunDirectory, func() error {
			var err error
			snapshots, err = linuxplatform.UpdateConfiguredSubscriptions(request.Context(), &http.Client{Timeout: 30 * time.Second}, app.ConfigPath, app.StateDirectory, parts[0])
			return err
		})
		if err != nil {
			writeWebError(writer, err, http.StatusUnprocessableEntity)
			return
		}
		writeWebJSON(writer, map[string]any{"snapshots": snapshots})
		return
	}
	if len(parts) == 3 && parts[1] == "nodes" && request.Method == http.MethodDelete {
		var snapshot linuxplatform.SubscriptionSnapshot
		err := withOperationLock(app.RunDirectory, func() error {
			var err error
			snapshot, err = linuxplatform.CleanSubscriptionNode(app.ConfigPath, app.StateDirectory, parts[0], parts[2])
			return err
		})
		if err != nil {
			writeWebError(writer, err, http.StatusUnprocessableEntity)
			return
		}
		writeWebJSON(writer, map[string]any{"snapshot": snapshot})
		return
	}
	http.NotFound(writer, request)
}

func (app webApplication) handleGeoData(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	kind := strings.TrimPrefix(request.URL.Path, "/api/v1/geodata/")
	names, err := geodata.Catalog(request.Context(), linuxplatform.ExecRunner{}, kind, "/usr/share/steer/geodata-seed", "/usr/bin/geoview")
	if err != nil {
		writeWebError(writer, err, http.StatusInternalServerError)
		return
	}
	writeWebJSON(writer, map[string]any{"kind": kind, "names": names})
}

func (app webApplication) applySaved() (coreapply.Result, error) {
	return runLockedApplyResult(app.RunDirectory, func() (coreapply.Result, error) {
		value, validation, err := loadIntent(app.ConfigPath)
		if err != nil {
			return coreapply.Result{Validation: &validation}, err
		}
		if !validation.OK {
			return coreapply.Result{Validation: &validation}, linuxplatform.ValidationError{Validation: validation}
		}
		return app.applyValue(value)
	})
}

func (app webApplication) applyValue(value model.Intent) (coreapply.Result, error) {
	validation := linuxplatform.Validate(value)
	if !validation.OK {
		return coreapply.Result{Validation: &validation}, linuxplatform.ValidationError{Validation: validation}
	}
	backend := linuxplatform.NewBackend(linuxplatform.ExecRunner{}, value, linuxplatform.BackendOptions{RunDirectory: app.RunDirectory, StateDirectory: app.StateDirectory})
	return coreapply.Run(context.Background(), value, backend.CompilerOptions(), backend)
}

func decodeIntentPayload(body []byte) (model.Intent, bool, error) {
	var wrapper struct {
		Intent json.RawMessage `json:"intent"`
		Apply  bool            `json:"apply"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil && len(wrapper.Intent) > 0 {
		value, err := model.DecodeJSON(bytes.NewReader(wrapper.Intent))
		return value, wrapper.Apply, err
	}
	value, err := model.DecodeJSON(bytes.NewReader(body))
	return value, false, err
}

func writeWebJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

func writeWebJSONStatus(writer http.ResponseWriter, value any, status int) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func writeWebError(writer http.ResponseWriter, err error, status int) {
	writeWebJSONStatus(writer, map[string]any{"error": err.Error()}, status)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	coreapply "github.com/gsh20040816/steer/go/internal/apply"
	"github.com/gsh20040816/steer/go/internal/geodata"
	linuxplatform "github.com/gsh20040816/steer/go/internal/platform/linux"
)

func (app webApplication) handleConfig(writer http.ResponseWriter, request *http.Request) {
	store := linuxplatform.IntentStore{Path: app.ConfigPath}
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

type webErrorDetails struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Kind     string `json:"kind,omitempty"`
	Category string `json:"category,omitempty"`
	Path     string `json:"path,omitempty"`
}

func errorDetails(err error) *webErrorDetails {
	if err == nil {
		return nil
	}
	details := &webErrorDetails{Code: "OPERATION_FAILED", Message: err.Error()}
	var geoErr *geodata.Error
	if errors.As(err, &geoErr) {
		details.Code = geoErr.Code
		details.Kind = geoErr.Kind
		details.Category = geoErr.Category
		details.Path = geoErr.Path
	}
	return details
}

func (app webApplication) platformPath() string {
	if app.PlatformPath == "" {
		return "/etc/steer/platform.json"
	}
	return app.PlatformPath
}

func (app webApplication) handlePlatform(writer http.ResponseWriter, request *http.Request) {
	store := linuxplatform.SettingsStore{Path: app.platformPath()}
	switch request.Method {
	case http.MethodGet:
		settings, revision, err := store.Load()
		if err != nil {
			writeWebError(writer, err, http.StatusInternalServerError)
			return
		}
		writer.Header().Set("ETag", revision)
		writeWebJSON(writer, map[string]any{"settings": settings, "revision": revision})
	case http.MethodPut:
		body, err := io.ReadAll(io.LimitReader(request.Body, 256<<10))
		if err != nil {
			writeWebError(writer, err, http.StatusBadRequest)
			return
		}
		settings, err := decodePlatformPayload(body)
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
				if _, statErr := os.Stat(app.platformPath()); statErr == nil {
					saveErr = errors.New("If-Match is required for existing platform settings")
					return nil
				}
			}
			revision, saveErr = store.Save(settings, expectedRevision)
			if saveErr != nil {
				return nil
			}
			value, validation, loadErr := loadIntent(app.ConfigPath)
			if loadErr != nil {
				applyResult, applyErr = coreapply.Result{Validation: &validation}, loadErr
			} else if !validation.OK {
				applyResult, applyErr = coreapply.Result{Validation: &validation}, linuxplatform.ValidationError{Validation: validation}
			} else {
				applyResult, applyErr = app.applyValueWithPlatform(value, settings)
			}
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
		response := map[string]any{"saved": true, "applied": applyErr == nil, "revision": revision, "apply_result": applyResult}
		if applyErr != nil {
			response["error"] = errorDetails(applyErr)
			writeWebJSONStatus(writer, response, http.StatusUnprocessableEntity)
			return
		}
		writeWebJSON(writer, response)
	default:
		writer.Header().Set("Allow", "GET, PUT")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func decodePlatformPayload(body []byte) (linuxplatform.Settings, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var value linuxplatform.Settings
	if err := decoder.Decode(&value); err != nil {
		return linuxplatform.Settings{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return linuxplatform.Settings{}, err
	}
	if err := linuxplatform.ValidateSettings(value); err != nil {
		return linuxplatform.Settings{}, err
	}
	return value, nil
}

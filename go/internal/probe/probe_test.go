// SPDX-License-Identifier: GPL-3.0-or-later

package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMeasureDownload(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte("payload"))
	}))
	defer server.Close()
	result := Measure(context.Background(), server.Client(), server.URL, true)
	if !result.OK || result.DownloadedBytes != 7 || result.Attempts != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestMeasureConnectionRetriesHTTPFailure(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	result := Measure(context.Background(), server.Client(), server.URL, false)
	if result.OK || result.Attempts != 2 || result.Status != http.StatusBadGateway {
		t.Fatalf("unexpected result: %#v", result)
	}
}

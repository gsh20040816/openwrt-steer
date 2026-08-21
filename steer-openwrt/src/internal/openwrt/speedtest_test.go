// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMeasureNodeSpeedTest(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte("fixture payload"))
	}))
	defer server.Close()
	result := measureNodeSpeedTest(context.Background(), server.Client(), server.URL, true)
	if result.Error != "" || result.Status != http.StatusOK || result.DownloadedBytes != int64(len("fixture payload")) || result.FirstByteMilliseconds < 0 {
		t.Fatalf("unexpected node speed test result: %#v", result)
	}
}

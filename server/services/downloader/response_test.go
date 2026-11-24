package downloader

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadWithHeadersRejectsUnsuccessfulResponse(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("not audio"))
	}))
	defer httpServer.Close()
	body, err := DownloadWithHeaders(httpServer.URL, nil)
	if err == nil || body != nil {
		t.Fatalf("bad HTTP response was accepted: %q %v", body, err)
	}
}

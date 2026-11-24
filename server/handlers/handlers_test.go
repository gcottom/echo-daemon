package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gcottom/echodaemon/services/downloader"
	"github.com/gin-gonic/gin"
)

func TestLocalHTTPContract(t *testing.T) {
	router := gin.New()
	SetupRoutes(router, &downloader.Service{})
	for _, tt := range []struct {
		p    string
		code int
		body string
	}{
		{"/healthz", 200, `{"status":"ok"}`},
		{"/download/invalid.id", 400, `{"error":"invalid track or playlist ID"}`},
	} {
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, tt.p, nil))
		if res.Code != tt.code || res.Body.String() != tt.body {
			t.Errorf("%s got %d %s", tt.p, res.Code, res.Body)
		}
	}
}

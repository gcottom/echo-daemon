package meta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2/clientcredentials"
)

func TestSpotifyFallbackKeepsSourceMetadata(t *testing.T) {
	expected := TrackMeta{Title: "Title", Artist: "Artist", ID: "Example1234"}
	service := &Service{}
	if result := service.enrichWithSpotify(context.Background(), expected); result.Title != expected.Title || result.ID != expected.ID {
		t.Fatalf("without config: %+v", result)
	}
	calls := make(chan struct{}, 1)
	badToken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case calls <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer badToken.Close()
	service.SpotifyConfig = &clientcredentials.Config{ClientID: "dummy", ClientSecret: "dummy", TokenURL: badToken.URL}
	got := service.enrichWithSpotify(context.Background(), expected)
	if got.Title != expected.Title || got.Artist != expected.Artist || got.ID != expected.ID {
		t.Fatalf("lost base metadata: %+v", got)
	}
	select {
	case <-calls:
	default:
		t.Fatal("expected a token request")
	}
}

package meta

import (
	"golang.org/x/oauth2/clientcredentials"
)

type Service struct {
	SpotifyConfig *clientcredentials.Config
}

type TrackMeta struct {
	ID          string `json:"id"`
	Status      string `json:"status,omitempty"`
	URL         string `json:"url,omitempty"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Album       string `json:"album,omitempty"`
	CoverArtURL string `json:"cover_art_url,omitempty"`
	Genre       string `json:"genre,omitempty"`
}

type YTMMetaResponse struct {
	Title  string `json:"title"`
	Author string `json:"author"`
	Image  string `json:"image"`
	Type   string `json:"type"`
}

type PlaylistResponse struct {
	Tracks []string `json:"tracks"`
}

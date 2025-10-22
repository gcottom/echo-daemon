package ytmusic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	ytmusicBaseURL = "https://music.youtube.com/youtubei/v1/"
	apiKey         = "AIzaSyC9XL3ZjWddXya6X74dJoCTL-WEYFDNX30"
	userAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

type Client struct {
	httpClient *http.Client
	context    map[string]interface{}
}

type VideoMetadata struct {
	Title  string
	Author string
	Image  string
}

type PlaylistResponse struct {
	Tracks []string
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		context: map[string]interface{}{
			"client": map[string]interface{}{
				"clientName":    "WEB_REMIX",
				"clientVersion": "1.20231204.01.00",
				"hl":            "en",
				"gl":            "US",
			},
		},
	}
}

func (c *Client) makeRequest(ctx context.Context, endpoint string, body interface{}) (map[string]interface{}, error) {
	url := ytmusicBaseURL + endpoint + "?key=" + apiKey

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Origin", "https://music.youtube.com")
	req.Header.Set("Referer", "https://music.youtube.com/")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyText, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(bodyText))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

func (c *Client) getPlayerResponse(ctx context.Context, videoID string) (map[string]interface{}, error) {
	requestBody := map[string]interface{}{
		"context": c.context,
		"videoId": videoID,
	}

	response, err := c.makeRequest(ctx, "player", requestBody)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (c *Client) GetVideoMetadata(ctx context.Context, videoID string) (*VideoMetadata, error) {
	playerResp, err := c.getPlayerResponse(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("failed to get player response: %w", err)
	}

	videoDetails, err := extractPath(playerResp, []string{"videoDetails"})
	if err != nil {
		return nil, fmt.Errorf("failed to extract video details: %w", err)
	}

	vd, ok := videoDetails.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("video details is not an object")
	}

	title, _ := vd["title"].(string)
	author, _ := vd["author"].(string)

	// Extract thumbnail URL
	var imageURL string
	if thumbnail, ok := vd["thumbnail"].(map[string]interface{}); ok {
		if thumbnails, ok := thumbnail["thumbnails"].([]interface{}); ok && len(thumbnails) > 0 {
			lastThumb := thumbnails[len(thumbnails)-1]
			if thumbObj, ok := lastThumb.(map[string]interface{}); ok {
				imageURL, _ = thumbObj["url"].(string)
			}
		}
	}

	return &VideoMetadata{
		Title:  title,
		Author: author,
		Image:  imageURL,
	}, nil
}

func (c *Client) GetPlaylist(ctx context.Context, playlistID string) (*PlaylistResponse, error) {
	requestBody := map[string]interface{}{
		"context":                       c.context,
		"browseId":                      "VL" + playlistID,
		"enablePersistentPlaylistPanel": true,
	}

	response, err := c.makeRequest(ctx, "browse", requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to get playlist: %w", err)
	}

	// Extract tracks from the playlist response
	tracks := []string{}

	// Try to find tracks in the response structure
	if contents, err := extractPath(response, []string{"contents", "singleColumnBrowseResultsRenderer", "tabs", "0", "tabRenderer", "content", "sectionListRenderer", "contents", "0", "musicPlaylistShelfRenderer", "contents"}); err == nil {
		if contentsList, ok := contents.([]interface{}); ok {
			for _, item := range contentsList {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if videoRenderer, ok := itemMap["musicResponsiveListItemRenderer"].(map[string]interface{}); ok {
						// Try to extract videoId from playlistItemData
						if playlistItemData, ok := videoRenderer["playlistItemData"].(map[string]interface{}); ok {
							if videoID, ok := playlistItemData["videoId"].(string); ok {
								tracks = append(tracks, videoID)
							}
						}
					}
				}
			}
		}
	}

	return &PlaylistResponse{
		Tracks: tracks,
	}, nil
}

func extractPath(data interface{}, path []string) (interface{}, error) {
	current := data
	for _, key := range path {
		switch v := current.(type) {
		case map[string]interface{}:
			var ok bool
			current, ok = v[key]
			if !ok {
				return nil, fmt.Errorf("key not found: %s", key)
			}
		case []interface{}:
			// Handle array index
			var idx int
			if _, err := fmt.Sscanf(key, "%d", &idx); err != nil {
				return nil, fmt.Errorf("invalid array index: %s", key)
			}
			if idx < 0 || idx >= len(v) {
				return nil, fmt.Errorf("array index out of bounds: %d", idx)
			}
			current = v[idx]
		default:
			return nil, fmt.Errorf("cannot traverse path, unexpected type at key: %s", key)
		}
	}
	return current, nil
}

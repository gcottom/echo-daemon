package downloader

import (
	"sync"

	"github.com/gcottom/echodaemon/services/meta"
)

type Service struct {
	MetaServiceClient *meta.Service
	LibraryMap        *sync.Map
}

type CaptureStartRequest struct {
	ID string `json:"trackId"`
}

type CaptureRequest struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Cookies string            `json:"cookies"`
	Body    string
}

type CurrentCapture struct {
	ID       string           `json:"id"`
	Requests []CaptureRequest `json:"requests"`
	Data     []byte           `json:"data"`
}

type CaptureChanData struct {
	IsStart        bool
	TrackID        string
	CaptureRequest *CaptureRequest
}

type SegData struct {
	C    int
	Data []byte
}

const MinimumDownloadSize = 1000000
const MB = 1024 * 1024

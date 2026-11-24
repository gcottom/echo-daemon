package downloader

import (
	"sync"

	"github.com/gcottom/echodaemon/services/meta"
)

type Service struct {
	MetaServiceClient *meta.Service
	LibraryMap        *sync.Map
	saveMu            sync.Mutex
	source            trackSource
}

package main

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gcottom/audiometa/v3"
	"github.com/gcottom/echodaemon/config"
	"github.com/gcottom/echodaemon/handlers"
	"github.com/gcottom/echodaemon/internal/genreengine"
	"github.com/gcottom/echodaemon/internal/ytmusic"
	"github.com/gcottom/echodaemon/logger"
	"github.com/gcottom/echodaemon/services/downloader"
	"github.com/gcottom/echodaemon/services/meta"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2/clientcredentials"
)

func main() {
	if err := RunServer(); err != nil {
		panic(err)
	}
}

func RunServer() error {
	ctx := logger.WithLogger(context.Background(), logger.DefaultLogger)
	logger.InfoC(ctx, "starting downloader server...")
	logger.InfoC(ctx, "loading config...")
	cfg, err := config.LoadConfigFromFile("")
	if err != nil {
		slog.Error("failed to load config", slog.Any("error", err))
		return err
	}

	logger.InfoC(ctx, "creating meta service...")
	// Initialize genre engine once and inject into meta service
	defGenre := "Electronic"
	if strings.TrimSpace(cfg.DefaultGenre) != "" {
		defGenre = cfg.DefaultGenre
	}
	engOpt := genreengine.Options{
		ModelDir:     "/app/models",
		DefaultGenre: defGenre,
		WorkDir:      "/tmp",
	}
	logger.InfoC(ctx, "initializing genre engine...")
	eng, engErr := genreengine.NewPreferredEngine(ctx, engOpt)
	if engErr != nil {
		logger.ErrorC(ctx, "genre engine initialization: falling back to heuristic", slog.Any("error", engErr))
	} else {
		logger.InfoC(ctx, "genre engine initialized")
	}
	metaService := &meta.Service{
		SpotifyConfig: &clientcredentials.Config{
			ClientID:     cfg.SpotifyClientID,
			ClientSecret: cfg.SpotifyClientSecret,
			TokenURL:     spotifyauth.TokenURL,
		},
		GenreLimiter:  make(chan struct{}, 1),
		Engine:        eng,
		YTMusicClient: ytmusic.NewClient(),
	}

	libMap := new(sync.Map)
	initLibraryMap(ctx, libMap, cfg.MusicDir)
	logger.InfoC(ctx, "creating downloader service...")
	downloaderService := &downloader.Service{
		MetaServiceClient: metaService,
		LibraryMap:        libMap,
	}

	logger.InfoC(ctx, "creating gin engine...")
	gin.SetMode(gin.ReleaseMode)
	ginws := gin.New()
	ginws.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Accept"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}),
		logger.Middleware(logger.DefaultLogger),
		gin.Recovery())

	logger.InfoC(ctx, "setting up routes...")
	handlers.SetupRoutes(ginws, downloaderService)
	logger.InfoC(ctx, "setup complete, starting server...")
	logger.InfoC(ctx, "now listening on port 50999!")
	logger.InfoC(ctx, "echo-daemon ready!")
	return http.ListenAndServe(":50999", ginws)
}

func initLibraryMap(ctx context.Context, libMap *sync.Map, musicDir string) {
	logger.InfoC(ctx, "initializing library map...")
	count := 0
	if err := filepath.WalkDir(musicDir, func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			logger.ErrorC(ctx, "error walking directory", slog.String("path", path), slog.Any("error", err))
			return err
		}
		if info.IsDir() {
			return nil // Skip directories
		}

		filename := filepath.Base(path)
		f, err := os.Open(path)
		if err != nil {
			logger.ErrorC(ctx, "failed to open file", slog.String("file", filename),
				slog.Any("error", err))
			return nil
		}
		defer func() { _ = f.Close() }()
		tag, err := audiometa.OpenTag(f)
		if err != nil {
			return nil // Skip files with no tags
		}
		if strings.TrimSpace(tag.GetTitle()) == "" || strings.TrimSpace(tag.GetArtist()) == "" {
			return nil // Skip files with an empty title or artist
		}
		count++
		libMap.Store(strings.TrimSpace(tag.GetTitle())+" - "+strings.TrimSpace(tag.GetArtist()), true)
		return nil
	}); err != nil {
		logger.ErrorC(ctx, "failed to initialize library map, failed to walk music directory", slog.String("musicDir", musicDir), slog.Any("error", err))
		return
	}
	logger.InfoC(ctx, "library map initialized", slog.Int("size", count))
}

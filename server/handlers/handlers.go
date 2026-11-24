package handlers

import (
	"errors"
	"github.com/gcottom/echodaemon/logger"
	"github.com/gcottom/echodaemon/services/downloader"
	"github.com/gin-gonic/gin"
	"log/slog"
)

type Handlers struct {
	Downloader *downloader.Service
}

func SetupRoutes(router *gin.Engine, downloaderService *downloader.Service) {
	handler := &Handlers{Downloader: downloaderService}
	router.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	router.GET("/download/:id", handler.Download)
}

func (h *Handlers) Download(ctx *gin.Context) {
	id := ctx.Param("id")
	if err := h.Downloader.DownloadByID(ctx.Request.Context(), id); err != nil {
		if errors.Is(err, downloader.ErrInvalidID) {
			ResponseFailure(ctx, err)
		} else {
			logger.ErrorC(ctx.Request.Context(), "could not start download", slog.String("id", id))
			ctx.AbortWithStatusJSON(502, Failure{Error: "could not start download"})
		}
		return
	}
	ResponseSuccess(ctx, StartDownloadResponse{State: "DOWNLOAD_INITIATED"})
}

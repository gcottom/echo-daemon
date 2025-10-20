package handlers

import (
	"github.com/gcottom/echodaemon/services/downloader"
	"github.com/gin-gonic/gin"
)

type Handlers struct {
	Downloader *downloader.Service
}

func SetupRoutes(router *gin.Engine, downloaderService *downloader.Service) {
	handler := &Handlers{Downloader: downloaderService}
	router.GET("/download/:id", handler.Download)
}

func (h *Handlers) Download(ctx *gin.Context) {
	id := ctx.Param("id")
	if err := h.Downloader.DownloadByID(ctx, id); err != nil {
		ResponseFailure(ctx, err)
		return
	}
	ResponseSuccess(ctx, StartDownloadResponse{State: "DOWNLOAD_INITIATED"})
}

package handlers

import (
	"github.com/VoidObscura/echodaemon/services/downloader"
	"github.com/gin-gonic/gin"
)

type Handlers struct {
	Downloader *downloader.Service
}

func SetupRoutes(router *gin.Engine, downloaderService *downloader.Service) {
	handler := &Handlers{Downloader: downloaderService}
	router.POST("/capturestart", handler.CaptureStart)
	router.POST("capture", handler.Capture)
}

func (h *Handlers) CaptureStart(ctx *gin.Context) {
	var reqData downloader.CaptureStartRequest
	ctx.ShouldBindJSON(&reqData)
	h.Downloader.NewCapture(ctx, reqData.ID)
	ResponseSuccess(ctx, StartDownloadResponse{State: "ACK"})

}

func (h *Handlers) Capture(ctx *gin.Context) {
	var reqData downloader.CaptureRequest
	ctx.ShouldBindJSON(&reqData)
	h.Downloader.ContinueCapture(ctx, reqData)
	ResponseSuccess(ctx, StartDownloadResponse{State: "ACK"})

}

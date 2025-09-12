package handlers

import "github.com/gin-gonic/gin"

type Failure struct {
	Error string `json:"error"`
}

type StartDownloadResponse struct {
	State string `json:"state"`
}

type StatusUpdate struct {
	ID                 string `json:"id"`
	Status             string `json:"status"`
	PlaylistTrackCount int    `json:"playlist_track_count,omitempty"`
	PlaylistTrackDone  int    `json:"playlist_track_done,omitempty"`
}

func ResponseFailure(ctx *gin.Context, err error) {
	ctx.AbortWithError(400, err)
}

func ResponseInternalError(ctx *gin.Context, err error) {
	ctx.AbortWithError(500, err)
}

func ResponseSuccess(ctx *gin.Context, data any) {
	ctx.JSON(200, data)
}

package handler

import (
	"database/sql"
	"log"
	db "messenger/internal/db/sqlc"
	"messenger/internal/storage"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type MediaHandler struct {
	minio   *storage.MinIOClient
	queries *db.Queries
}

func NewMediaHandler(minio *storage.MinIOClient, queries *db.Queries) *MediaHandler {
	return &MediaHandler{minio: minio, queries: queries}
}

var allowedMINE = map[string]mediaInfo{
	".jpg":  {mime: "image/jpeg", mediaType: db.MediaTypeImage},
	".jpeg": {mime: "image/jpeg", mediaType: db.MediaTypeImage},
	".png":  {mime: "image/png", mediaType: db.MediaTypeImage},
	".webp": {mime: "image/webp", mediaType: db.MediaTypeImage},
	".gif":  {mime: "image/gif", mediaType: db.MediaTypeGif},
	".mp4":  {mime: "video/mp4", mediaType: db.MediaTypeVideo},
	".mov":  {mime: "video/quicktime", mediaType: db.MediaTypeVideo},
	".mp3":  {mime: "audio/mpeg", mediaType: db.MediaTypeAudio},
	".ogg":  {mime: "audio/ogg", mediaType: db.MediaTypeAudio},
	".pdf":  {mime: "application/pdf", mediaType: db.MediaTypeFile},
	".zip":  {mime: "application/zip", mediaType: db.MediaTypeFile},
}

type mediaInfo struct {
	mime      string
	mediaType db.MediaType
}

type UploadRequest struct {
	Filename string `json:"filename" binding:"required"`
	MimeType string `json:"mime_type" binding:"required"`
}
type UploadResponse struct {
	MediaID   string `json:"media_id"`
	UploadURL string `json:"upload_url"`
	ObjectKey string `json:"object_key"`
	ExpiresIn int    `json:"expires_in"`
}

func (h *MediaHandler) RequestUpload(c *gin.Context) {
	var req UploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "filename and mime_type obligatory"})
		return
	}
	ext := strings.ToLower(filepath.Ext(req.Filename))
	info, ok := allowedMINE[ext]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type file unresolved"})
		return
	}
	userID, ok := c.MustGet("user_id").(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user"})
		return
	}
	userIDStr := userID.String()

	objectKey := h.minio.GenerateObjectKey(userIDStr, req.Filename)

	media, err := h.queries.CreateMedia(c.Request.Context(), db.CreateMediaParams{
		UploaderID:   userID,
		Type:         info.mediaType,
		Bucket:       "messenger-media",
		ObjectKey:    objectKey,
		OriginalName: sql.NullString{String: req.Filename, Valid: true},
		MimeType:     info.mime,
	})
	if err != nil {
		log.Printf("❌ CreateMedia error: %v", err) // ← добавь это

		c.JSON(http.StatusInternalServerError, gin.H{"error": "error saving in BD"})
		return
	}
	expiry := 15 * time.Minute
	uploadURL, err := h.minio.PresignedPutURL(c.Request.Context(), objectKey, expiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed created URL for loading"})
		return
	}
	c.JSON(http.StatusOK, UploadResponse{
		MediaID:   media.ID.String(),
		UploadURL: uploadURL,
		ObjectKey: objectKey,
		ExpiresIn: int(expiry.Seconds()),
	})
}

type ConfirmRequest struct {
	MediaID     string  `json:"media_id" binding:"required"`
	SizeBytes   int64   `json:"size_bytes"`
	Width       int32   `json:"width"`
	Height      int32   `json:"height"`
	DurationSec float64 `json:"duration_sec"`
}

func (h *MediaHandler) ConfirmUpload(c *gin.Context) {
	var req ConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "media_id neccessarily"})
		return
	}

	mediaID, err := uuid.Parse(req.MediaID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid media_id"})
		return
	}

	media, err := h.queries.UpdateMediaUploaded(c.Request.Context(), db.UpdateMediaUploadedParams{
		ID:          mediaID,
		SizeBytes:   sql.NullInt64{Int64: req.SizeBytes, Valid: req.SizeBytes > 0},
		Width:       sql.NullInt32{Int32: req.Width, Valid: req.Height > 0},
		Height:      sql.NullInt32{Int32: req.Height, Valid: req.Height > 0},
		DurationSec: sql.NullFloat64{Float64: req.DurationSec, Valid: req.DurationSec > 0},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "error updated status"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"media_id":  media.ID.String(),
		"status":    media.Status,
		"url":       h.minio.PublicURL(media.ObjectKey),
		"thumb_url": h.minio.PublicURL(media.ThumbKey.String),
	})
}

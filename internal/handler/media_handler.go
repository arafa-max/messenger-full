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
    // ── Изображения ──────────────────────────────────────────
    ".jpg":  {mime: "image/jpeg", mediaType: db.MediaTypeImage},
    ".jpeg": {mime: "image/jpeg", mediaType: db.MediaTypeImage},
    ".png":  {mime: "image/png", mediaType: db.MediaTypeImage},
    ".webp": {mime: "image/webp", mediaType: db.MediaTypeImage},
    ".gif":  {mime: "image/gif", mediaType: db.MediaTypeGif},
    ".heic": {mime: "image/heic", mediaType: db.MediaTypeImage},
    ".heif": {mime: "image/heif", mediaType: db.MediaTypeImage},
    ".svg":  {mime: "image/svg+xml", mediaType: db.MediaTypeImage},
    ".bmp":  {mime: "image/bmp", mediaType: db.MediaTypeImage},
    ".tiff": {mime: "image/tiff", mediaType: db.MediaTypeImage},
    ".tif":  {mime: "image/tiff", mediaType: db.MediaTypeImage},
    ".ico":  {mime: "image/x-icon", mediaType: db.MediaTypeImage},

    // ── Видео ────────────────────────────────────────────────
    ".mp4":  {mime: "video/mp4", mediaType: db.MediaTypeVideo},
    ".mov":  {mime: "video/quicktime", mediaType: db.MediaTypeVideo},
    ".webm": {mime: "video/webm", mediaType: db.MediaTypeVideo},
    ".avi":  {mime: "video/x-msvideo", mediaType: db.MediaTypeVideo},
    ".mkv":  {mime: "video/x-matroska", mediaType: db.MediaTypeVideo},
    ".flv":  {mime: "video/x-flv", mediaType: db.MediaTypeVideo},
    ".wmv":  {mime: "video/x-ms-wmv", mediaType: db.MediaTypeVideo},
    ".m4v":  {mime: "video/x-m4v", mediaType: db.MediaTypeVideo},
    ".3gp":  {mime: "video/3gpp", mediaType: db.MediaTypeVideo},
    ".ts":   {mime: "video/mp2t", mediaType: db.MediaTypeVideo},

    // ── Аудио ────────────────────────────────────────────────
    ".mp3":  {mime: "audio/mpeg", mediaType: db.MediaTypeAudio},
    ".ogg":  {mime: "audio/ogg", mediaType: db.MediaTypeAudio},
    ".opus": {mime: "audio/opus", mediaType: db.MediaTypeAudio},
    ".m4a":  {mime: "audio/mp4", mediaType: db.MediaTypeAudio},
    ".wav":  {mime: "audio/wav", mediaType: db.MediaTypeAudio},
    ".flac": {mime: "audio/flac", mediaType: db.MediaTypeAudio},
    ".aac":  {mime: "audio/aac", mediaType: db.MediaTypeAudio},
    ".weba": {mime: "audio/webm", mediaType: db.MediaTypeAudio},
    ".aiff": {mime: "audio/aiff", mediaType: db.MediaTypeAudio},
    ".amr":  {mime: "audio/amr", mediaType: db.MediaTypeAudio},

    // ── Документы ────────────────────────────────────────────
    ".pdf":  {mime: "application/pdf", mediaType: db.MediaTypeFile},
    ".doc":  {mime: "application/msword", mediaType: db.MediaTypeFile},
    ".docx": {mime: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", mediaType: db.MediaTypeFile},
    ".xls":  {mime: "application/vnd.ms-excel", mediaType: db.MediaTypeFile},
    ".xlsx": {mime: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", mediaType: db.MediaTypeFile},
    ".ppt":  {mime: "application/vnd.ms-powerpoint", mediaType: db.MediaTypeFile},
    ".pptx": {mime: "application/vnd.openxmlformats-officedocument.presentationml.presentation", mediaType: db.MediaTypeFile},
    ".txt":  {mime: "text/plain", mediaType: db.MediaTypeFile},
    ".csv":  {mime: "text/csv", mediaType: db.MediaTypeFile},
    ".rtf":  {mime: "application/rtf", mediaType: db.MediaTypeFile},
    ".epub": {mime: "application/epub+zip", mediaType: db.MediaTypeFile},
    ".odt":  {mime: "application/vnd.oasis.opendocument.text", mediaType: db.MediaTypeFile},
    ".ods":  {mime: "application/vnd.oasis.opendocument.spreadsheet", mediaType: db.MediaTypeFile},
    ".odp":  {mime: "application/vnd.oasis.opendocument.presentation", mediaType: db.MediaTypeFile},

    // ── Архивы ───────────────────────────────────────────────
    ".zip":  {mime: "application/zip", mediaType: db.MediaTypeFile},
    ".rar":  {mime: "application/vnd.rar", mediaType: db.MediaTypeFile},
    ".7z":   {mime: "application/x-7z-compressed", mediaType: db.MediaTypeFile},
    ".tar":  {mime: "application/x-tar", mediaType: db.MediaTypeFile},
    ".gz":   {mime: "application/gzip", mediaType: db.MediaTypeFile},
    ".bz2":  {mime: "application/x-bzip2", mediaType: db.MediaTypeFile},
    ".xz":   {mime: "application/x-xz", mediaType: db.MediaTypeFile},

    // ── Код и данные ─────────────────────────────────────────
    ".json": {mime: "application/json", mediaType: db.MediaTypeFile},
    ".xml":  {mime: "application/xml", mediaType: db.MediaTypeFile},
    ".html": {mime: "text/html", mediaType: db.MediaTypeFile},
    ".css":  {mime: "text/css", mediaType: db.MediaTypeFile},
    ".js":   {mime: "text/javascript", mediaType: db.MediaTypeFile},
    ".py":   {mime: "text/x-python", mediaType: db.MediaTypeFile},
    ".go":   {mime: "text/x-go", mediaType: db.MediaTypeFile},
    ".md":   {mime: "text/markdown", mediaType: db.MediaTypeFile},
    ".yaml": {mime: "application/yaml", mediaType: db.MediaTypeFile},
    ".yml":  {mime: "application/yaml", mediaType: db.MediaTypeFile},
    ".toml": {mime: "application/toml", mediaType: db.MediaTypeFile},
    ".sh":   {mime: "application/x-sh", mediaType: db.MediaTypeFile},
    ".sql":  {mime: "application/sql", mediaType: db.MediaTypeFile},

    // ── Другое ───────────────────────────────────────────────
    ".apk":  {mime: "application/vnd.android.package-archive", mediaType: db.MediaTypeFile},
    ".ipa":  {mime: "application/octet-stream", mediaType: db.MediaTypeFile},
    ".torrent": {mime: "application/x-bittorrent", mediaType: db.MediaTypeFile},
    ".ttf":  {mime: "font/ttf", mediaType: db.MediaTypeFile},
    ".otf":  {mime: "font/otf", mediaType: db.MediaTypeFile},
    ".woff": {mime: "font/woff", mediaType: db.MediaTypeFile},
    ".woff2":{mime: "font/woff2", mediaType: db.MediaTypeFile},
}

type mediaInfo struct {
	mime      string
	mediaType db.MediaType
}

type UploadRequest struct {
	Filename  string `json:"filename" binding:"required"`
	MimeType  string `json:"mime_type" binding:"required"`
	SizeBytes int64  `json:"size_bytes"`
}
type UploadResponse struct {
	MediaID   string `json:"media_id"`
	UploadURL string `json:"upload_url"`
	ObjectKey string `json:"object_key"`
	ExpiresIn int    `json:"expires_in"`
}

const (
	limitFree     = 2 * 1024 * 1024 * 1024  // 2GB
	limitPremium  = 4 * 1024 * 1024 * 1024  // 4GB
	limitBusiness = 10 * 1024 * 1024 * 1024 // 10GB
)

func getFileSizeLimit(plan string) int64 {
	switch plan {
	case "premium":
		return limitPremium
	case "business":
		return limitBusiness
	default:
		return limitFree
	}
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
	// Проверяем лимит размера файла
	if req.SizeBytes > 0 {
		plan := "free"
		if sub, err := h.queries.GetSubscriptionByUserID(c, userID); err == nil && sub.Status == "active" {
			plan = sub.Plan
		}
		limit := getFileSizeLimit(plan)
		if req.SizeBytes > limit {
			c.JSON(http.StatusPaymentRequired, gin.H{
				"error": "file too large for your plan",
				"limit": limit,
				"plan":  plan,
			})
			return
		}
	}
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

package handler

import (
	"database/sql"
	"messenger/internal/ocr"
	"net/http"
	"path/filepath"
	"strings"

	db "messenger/internal/db/sqlc"
	"messenger/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type OCRHandler struct {
	queries *db.Queries
	storage *storage.MinIOClient
	ocr     *ocr.Client
}

func NewOCRHandler(queries *db.Queries, storage *storage.MinIOClient, ocrClient *ocr.Client) *OCRHandler {
	return &OCRHandler{
		queries: queries,
		storage: storage,
		ocr:     ocrClient,
	}
}

// POST /api/v1/media/:id/ocr
// Запускает OCR на изображении и сохраняет результат в media_search_index
// @Summary      Run OCR on image
// @Tags         media
// @Security     BearerAuth
// @Param        id   path  string  true  "Media ID"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Router       /media/{id}/ocr [post]
func (h *OCRHandler) RunOCR(c *gin.Context) {
	if !h.ocr.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "OCR service unavailable — tesseract not installed",
		})
		return
	}

	mediaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid media id"})
		return
	}

	media, err := h.queries.GetMediaByID(c, mediaID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "media not found"})
		return
	}

	// OCR только для изображений
	if media.Type != db.MediaTypeImage {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OCR only supported for images"})
		return
	}

	// Проверяем не индексировали ли уже
	existing, err := h.queries.GetMediaSearchIndex(c, mediaID)
	if err == nil && existing.Content != "" {
		c.JSON(http.StatusOK, gin.H{
			"text":   existing.Content,
			"cached": true,
		})
		return
	}

	// Скачиваем изображение из MinIO
	imageData, err := h.storage.Download(c.Request.Context(), media.ObjectKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to download image"})
		return
	}

	// Определяем расширение для tmp файла
	ext := strings.ToLower(filepath.Ext(media.ObjectKey))
	if ext == "" {
		ext = ".jpg"
	}

	// Запускаем OCR
	text, err := h.ocr.ExtractText(c.Request.Context(), imageData, ext)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "OCR failed: " + err.Error()})
		return
	}

	if text == "" {
		c.JSON(http.StatusOK, gin.H{
			"text":    "",
			"message": "no text found in image",
		})
		return
	}

	// Сохраняем в поисковый индекс
	_ = h.queries.CreateMediaSearchIndex(c, db.CreateMediaSearchIndexParams{
		MediaID:   mediaID,
		Content:   text,
		Source:    "ocr",
		Lang:      sql.NullString{String: "ru", Valid: true},
		MessageID: uuid.NullUUID{},
	})

	c.JSON(http.StatusOK, gin.H{
		"text":     text,
		"media_id": mediaID.String(),
	})
}

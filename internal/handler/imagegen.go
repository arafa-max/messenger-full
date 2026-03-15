package handler

import (
	"net/http"

	"messenger/internal/ai"
	"messenger/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ImageGenHandler struct {
	sd      ai.ImageGenerator
	storage *storage.MinIOClient
}

func NewImageGenHandler(sd ai.ImageGenerator, storage *storage.MinIOClient) *ImageGenHandler {
	return &ImageGenHandler{sd: sd, storage: storage}
}

// POST /ai/generate-image
func (h *ImageGenHandler) Generate(c *gin.Context) {
	var req struct {
		Prompt string `json:"prompt" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Генерируем картинку
	imgBytes, err := h.sd.Generate(c, req.Prompt)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "image generation unavailable",
		})
		return
	}

	// Сохраняем в MinIO
	fileID := uuid.New().String() + ".png"
	url, err := h.storage.UploadBytes(c, "generated-images", fileID, imgBytes, "image/png")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save image"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":    url,
		"prompt": req.Prompt,
	})
}
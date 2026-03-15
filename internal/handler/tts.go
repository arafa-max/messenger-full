package handler

import (
	"net/http"

	"messenger/internal/ai"
	"messenger/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type TTSHandler struct {
	tts     ai.TTSClient
	storage *storage.MinIOClient
}

func NewTTSHandler(tts ai.TTSClient, storage *storage.MinIOClient) *TTSHandler {
	return &TTSHandler{tts: tts, storage: storage}
}

// POST /ai/tts — текст в голос
func (h *TTSHandler) Synthesize(c *gin.Context) {
	var req struct {
		Text string `json:"text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len([]rune(req.Text)) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "text too long, max 500 chars"})
		return
	}

	audioBytes, err := h.tts.Synthesize(c, req.Text, "")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "TTS unavailable",
		})
		return
	}

	// Сохраняем в MinIO
	fileID := uuid.New().String() + ".wav"
	url, err := h.storage.UploadBytes(c, "tts-audio", fileID, audioBytes, "audio/wav")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save audio"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":  url,
		"text": req.Text,
	})
}
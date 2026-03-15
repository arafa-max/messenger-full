package handler

import (
	"net/http"

	"messenger/internal/ai"

	"github.com/gin-gonic/gin"
)

type WhisperHandler struct {
	transcriber ai.Transcriber
}

func NewWhisperHandler(transcriber ai.Transcriber) *WhisperHandler {
	return &WhisperHandler{transcriber: transcriber}
}

func (h *WhisperHandler) Transcribe(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
		return
	}
	defer file.Close()

	if header.Size > 25<<20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file too large"})
		return
	}

	buf := make([]byte, header.Size)
	if _, err := file.Read(buf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	result, err := h.transcriber.Transcribe(c, buf, header.Filename)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "transcription unavailable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"text":          result.Text,
		"language":      result.Language,
		"language_prob": result.LanguageProb,
		"duration":      result.Duration,
	})
}

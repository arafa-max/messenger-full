package handler

import (
	"net/http"

	"messenger/internal/ai"

	"github.com/gin-gonic/gin"
)

type TranslateHandler struct {
	translator ai.Translator
}

func NewTranslateHandler(translator ai.Translator) *TranslateHandler {
	return &TranslateHandler{translator: translator}
}

// POST /ai/translate — перевод текста
func (h *TranslateHandler) Translate(c *gin.Context) {
	var req struct {
		Text       string `json:"text"        binding:"required"`
		TargetLang string `json:"target_lang" binding:"required"`
		SourceLang string `json:"source_lang"` // пусто = автоопределение
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	translated, err := h.translator.Translate(c, req.Text, req.SourceLang, req.TargetLang)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "translation unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"translated":  translated,
		"source_lang": req.SourceLang,
		"target_lang": req.TargetLang,
	})
}

// POST /ai/detect-language — определение языка
func (h *TranslateHandler) DetectLanguage(c *gin.Context) {
	var req struct {
		Text string `json:"text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	lang, err := h.translator.DetectLanguage(c, req.Text)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "detection unavailable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"language": lang})
}
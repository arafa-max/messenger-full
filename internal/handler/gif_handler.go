package handler

import (
	"net/http"

	"messenger/internal/gif"

	"github.com/gin-gonic/gin"
)

type GifHandler struct {
	service *gif.Service
}

func NewGifHandler(s *gif.Service) *GifHandler {
	return &GifHandler{service: s}
}

func (h *GifHandler) Search(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query required"})
		return
	}
	gifs, err := h.service.Search(c.Request.Context(), q, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gifs)
}

func (h *GifHandler) Trending(c *gin.Context) {
	gifs, err := h.service.Trending(c.Request.Context(), 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gifs)
}
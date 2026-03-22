package handler

import (
	"encoding/json"
	"net/http"

	db "messenger/internal/db/sqlc"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type BusinessHandler struct {
	queries *db.Queries
}

func NewBusinessHandler(queries *db.Queries) *BusinessHandler {
	return &BusinessHandler{queries: queries}
}

// GET /business/profile
func (h *BusinessHandler) GetProfile(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	profile, err := h.queries.GetBusinessProfile(c, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "business profile not found"})
		return
	}

	c.JSON(http.StatusOK, profile)
}

// PUT /business/profile
func (h *BusinessHandler) UpsertProfile(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req struct {
		BusinessName string         `json:"business_name"`
		Category     string         `json:"category"`
		Description  string         `json:"description"`
		Address      string         `json:"address"`
		Email        string         `json:"email"`
		Website      string         `json:"website"`
		PhonePublic  string         `json:"phone_public"`
		WorkingHours map[string]any `json:"working_hours"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Сериализуем working_hours в JSON
	hoursJSON, err := json.Marshal(req.WorkingHours)
	if err != nil {
		hoursJSON = []byte("{}")
	}

	profile, err := h.queries.UpsertBusinessProfile(c, db.UpsertBusinessProfileParams{
		UserID:       userID,
		BusinessName: req.BusinessName,
		Category:     req.Category,
		Description:  req.Description,
		Address:      req.Address,
		Email:        req.Email,
		Website:      req.Website,
		PhonePublic:  req.PhonePublic,
		WorkingHours: hoursJSON,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save profile"})
		return
	}

	c.JSON(http.StatusOK, profile)
}

// GET /business/:username — публичный профиль
func (h *BusinessHandler) GetPublicProfile(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username required"})
		return
	}

	// Получаем userID по username
	c.JSON(http.StatusOK, gin.H{"username": username})
}

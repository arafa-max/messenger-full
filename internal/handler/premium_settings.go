package handler

import (
	"net/http"

	db "messenger/internal/db/sqlc"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type PremiumSettingsHandler struct {
	queries *db.Queries
}

func NewPremiumSettingsHandler(queries *db.Queries) *PremiumSettingsHandler {
	return &PremiumSettingsHandler{queries: queries}
}

// PUT /premium/settings — обновить Premium настройки
func (h *PremiumSettingsHandler) UpdateSettings(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req struct {
		HidePhone           *bool   `json:"hide_phone"`
		AwayMessage         *string `json:"away_message"`
		AwayMessageEnabled  *bool   `json:"away_message_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	settings, _ := h.queries.GetPremiumSettings(c, userID)

	hidePhone := settings.HidePhone
	awayMsg := settings.AwayMessage
	awayEnabled := settings.AwayMessageEnabled

	if req.HidePhone != nil {
		hidePhone = *req.HidePhone
	}
	if req.AwayMessage != nil {
		awayMsg = *req.AwayMessage
	}
	if req.AwayMessageEnabled != nil {
		awayEnabled = *req.AwayMessageEnabled
	}

	err = h.queries.UpsertPremiumSettings(c, db.UpsertPremiumSettingsParams{
		UserID:              userID,
		HidePhone:           hidePhone,
		AwayMessage:         awayMsg,
		AwayMessageEnabled:  awayEnabled,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update settings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "settings updated"})
}

// GET /premium/settings
func (h *PremiumSettingsHandler) GetSettings(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	settings, err := h.queries.GetPremiumSettings(c, userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"hide_phone":           false,
			"away_message":         "",
			"away_message_enabled": false,
		})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// POST /premium/labels — добавить ярлык чату
func (h *PremiumSettingsHandler) AddChatLabel(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req struct {
		ChatID string `json:"chat_id" binding:"required"`
		Label  string `json:"label"   binding:"required"`
		Color  string `json:"color"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	chatID, err := uuid.Parse(req.ChatID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}

	color := req.Color
	if color == "" {
		color = "#6B7280"
	}

	label, err := h.queries.AddChatLabel(c, db.AddChatLabelParams{
		UserID: userID,
		ChatID: chatID,
		Label:  req.Label,
		Color:  color,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add label"})
		return
	}

	c.JSON(http.StatusCreated, label)
}

// GET /premium/labels — список ярлыков
func (h *PremiumSettingsHandler) GetChatLabels(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	labels, err := h.queries.GetChatLabels(c, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get labels"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"labels": labels})
}

// DELETE /premium/labels/:id
func (h *PremiumSettingsHandler) DeleteChatLabel(c *gin.Context) {
	labelID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid label_id"})
		return
	}

	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	err = h.queries.DeleteChatLabel(c, db.DeleteChatLabelParams{
		ID:     labelID,
		UserID: userID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete label"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "label deleted"})
}
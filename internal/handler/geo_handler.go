package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type GeoHandler struct {
	db  *sql.DB
	hub *WSHandler
}

func NewGeoHandler(db *sql.DB, hub *WSHandler) *GeoHandler {
	return &GeoHandler{db: db, hub: hub}
}

type SendGeoRequest struct {
	Lat      float64 `json:"lat" binding:"required"`
	Lng      float64 `json:"lng" binding:"required"`
	Live     bool    `json:"live"`
	Duration int     `json:"duration_minutes"`
}

func (h *GeoHandler) SendLocation(c *gin.Context) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	var req SendGeoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	geoData := map[string]any{
		"lat":  req.Lat,
		"lng":  req.Lng,
		"live": req.Live,
	}

	var expiresAt *time.Time
	if req.Live && req.Duration > 0 {
		t := time.Now().Add(time.Duration(req.Duration) * time.Minute)
		expiresAt = &t
		geoData["expires_at"] = t
	}

	var msgID uuid.UUID
	err = h.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO messages (chat_id, sender_id, type, content, geo)
		VALUES ($1, $2, 'geo', '', $3)
		RETURNING id
	`, chatID, userID, geoData).Scan(&msgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if req.Live && expiresAt != nil {
		h.db.ExecContext(c.Request.Context(), `
			INSERT INTO live_locations (user_id, chat_id, message_id, latitude, longitude, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, userID, chatID, msgID, req.Lat, req.Lng, expiresAt)
	}

	c.JSON(http.StatusOK, gin.H{"message_id": msgID})
}

func (h *GeoHandler) UpdateLiveLocation(c *gin.Context) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	var req struct {
		Lat float64 `json:"lat" binding:"required"`
		Lng float64 `json:"lng" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.db.ExecContext(c.Request.Context(), `
		UPDATE live_locations
		SET latitude = $1, longitude = $2, updated_at = NOW()
		WHERE user_id = $3 AND chat_id = $4 AND expires_at > NOW()
	`, req.Lat, req.Lng, userID, chatID)

	payload, _ := json.Marshal(map[string]any{
		"user_id": userID.String(),
		"lat":     req.Lat,
		"lng":     req.Lng,
	})

	h.hub.broadcastToChat(chatID.String(), &WSMessage{
		Type:    "live_location",
		Payload: payload,
		ChatID:  chatID.String(),
		TS:      time.Now().UnixMilli(),
	}, userID.String())

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *GeoHandler) StopLiveLocation(c *gin.Context) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	h.db.ExecContext(c.Request.Context(), `
		UPDATE live_locations
		SET expires_at = NOW()
		WHERE user_id = $1 AND chat_id = $2
	`, userID, chatID)

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

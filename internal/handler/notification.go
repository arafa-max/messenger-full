package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	db "messenger/internal/db/sqlc"
	"messenger/internal/push"
)

type NotificationHandler struct {
	q    *db.Queries
	push *push.Client
}

func NewNotificationHandler(sqlDB *sql.DB, pushClient *push.Client) *NotificationHandler {
	return &NotificationHandler{
		q:    db.New(sqlDB),
		push: pushClient,
	}
}

// ── GET /api/v1/notifications ─────────────────────────────────────────────────

func (h *NotificationHandler) GetNotifications(c *gin.Context) {
	userID := c.MustGet("user_id").(uuid.UUID)

	limit := int32(20)
	offset := int32(0)

	notifications, err := h.q.GetUserNotifications(c, db.GetUserNotificationsParams{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	count, _ := h.q.GetUnreadNotificationsCount(c, userID)
	c.JSON(http.StatusOK, gin.H{
		"notifications": notifications,
		"unread_count":  count,
	})
}

// ── POST /api/v1/notifications/:id/read ───────────────────────────────────────

func (h *NotificationHandler) MarkRead(c *gin.Context) {
	userID := c.MustGet("user_id").(uuid.UUID)
	notifID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification_id"})
		return
	}

	if err := h.q.MarkNotificationRead(c, db.MarkNotificationReadParams{
		ID:     notifID,
		UserID: userID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ── POST /api/v1/notifications/read-all ───────────────────────────────────────

func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	userID := c.MustGet("user_id").(uuid.UUID)

	if err := h.q.MarkAllNotificationsRead(c, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark all read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ── POST /api/v1/devices ──────────────────────────────────────────────────────

type registerDeviceReq struct {
	PushToken  string `json:"push_token" binding:"required"`
	Platform   string `json:"platform" binding:"required"` // android, ios, web
	DeviceName string `json:"device_name"`
}

func (h *NotificationHandler) RegisterDevice(c *gin.Context) {
	userID := c.MustGet("user_id").(uuid.UUID)

	var req registerDeviceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	device, err := h.q.CreateDevice(c, db.CreateDeviceParams{
		UserID:     userID,
		PushToken:  sql.NullString{String: req.PushToken, Valid: true},
		Platform:   req.Platform,
		DeviceName: sql.NullString{String: req.DeviceName, Valid: req.DeviceName != ""},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register device"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"device_id": device.ID})
}

// ── DELETE /api/v1/devices/:id ────────────────────────────────────────────────

func (h *NotificationHandler) UnregisterDevice(c *gin.Context) {
	deviceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device_id"})
		return
	}

	if err := h.q.DeactivateDevice(c, deviceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unregister device"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "unregistered"})
}

// ── SendPush — внутренний метод для отправки пуша пользователю ────────────────

func (h *NotificationHandler) SendPush(c *gin.Context, userID uuid.UUID, title, body, notifType string, refID *uuid.UUID) {
	// Сохраняем в БД
	var refNullID uuid.NullUUID
	if refID != nil {
		refNullID = uuid.NullUUID{UUID: *refID, Valid: true}
	}

	_, _ = h.q.CreateNotification(c, db.CreateNotificationParams{
		UserID:      userID,
		Type:        notifType,
		Title:       sql.NullString{String: title, Valid: true},
		Body:        sql.NullString{String: body, Valid: true},
		ReferenceID: refNullID,
	})

	// Отправляем пуш если FCM настроен
	if !h.push.IsConfigured() {
		return
	}

	tokens, err := h.q.GetUserPushTokens(c, userID)
	if err != nil || len(tokens) == 0 {
		return
	}

	var targets []push.TokenPlatform
	for _, t := range tokens {
		if t.PushToken.Valid {
			targets = append(targets, push.TokenPlatform{
				Token:    t.PushToken.String,
				Platform: push.Platform(t.Platform),
			})
		}
	}

	h.push.SendMulti(c, targets, push.Notification{
		Title: title,
		Body:  body,
	})
}
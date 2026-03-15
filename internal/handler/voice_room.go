package handler

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	db "messenger/internal/db/sqlc"
)

type VoiceRoomHandler struct {
	q   *db.Queries
	sql *sql.DB
	ws  *WSHandler
}

func NewVoiceRoomHandler(sqlDB *sql.DB, ws *WSHandler) *VoiceRoomHandler {
	return &VoiceRoomHandler{
		q:   db.New(sqlDB),
		sql: sqlDB,
		ws:  ws,
	}
}

// ─────────────────────────────────────────────
// POST /api/v1/chats/:id/voice-rooms
// Создать голосовую комнату в чате
// ─────────────────────────────────────────────

type createVoiceRoomReq struct {
	Name      string `json:"name" binding:"required,min=1,max=100"`
	Type      string `json:"type"` // voice, stage, video
	UserLimit int    `json:"user_limit"`
}

func (h *VoiceRoomHandler) CreateRoom(c *gin.Context) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}

	var req createVoiceRoomReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	roomType := req.Type
	if roomType == "" {
		roomType = "voice"
	}

	roomID := uuid.New()
	_, err = h.sql.ExecContext(c, `
		INSERT INTO voice_rooms (id, chat_id, name, type, user_limit, is_active)
		VALUES ($1, $2, $3, $4, $5, TRUE)
	`, roomID, chatID, req.Name, roomType, req.UserLimit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create voice room"})
		return
	}

	// Уведомляем участников чата о новой комнате
	h.ws.publishToChat(chatID.String(), &WSMessage{
		Type:   "voice_room.created",
		ChatID: chatID.String(),
		TS:     time.Now().UnixMilli(),
	}, "")

	c.JSON(http.StatusCreated, gin.H{
		"id":         roomID,
		"chat_id":    chatID,
		"name":       req.Name,
		"type":       roomType,
		"user_limit": req.UserLimit,
	})
}

// ─────────────────────────────────────────────
// GET /api/v1/chats/:id/voice-rooms
// Список голосовых комнат чата
// ─────────────────────────────────────────────

func (h *VoiceRoomHandler) GetRooms(c *gin.Context) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}

	rows, err := h.sql.QueryContext(c, `
		SELECT vr.id, vr.name, vr.type, vr.user_limit, vr.is_active, vr.created_at,
		       COUNT(vrp.user_id) AS participant_count
		FROM voice_rooms vr
		LEFT JOIN voice_room_participants vrp ON vrp.room_id = vr.id
		WHERE vr.chat_id = $1 AND vr.is_active = TRUE
		GROUP BY vr.id, vr.name, vr.type, vr.user_limit, vr.is_active, vr.created_at
		ORDER BY vr.created_at ASC
	`, chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type roomRow struct {
		ID               string    `json:"id"`
		Name             string    `json:"name"`
		Type             string    `json:"type"`
		UserLimit        int       `json:"user_limit"`
		IsActive         bool      `json:"is_active"`
		CreatedAt        time.Time `json:"created_at"`
		ParticipantCount int       `json:"participant_count"`
	}

	var rooms []roomRow
	for rows.Next() {
		var r roomRow
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.UserLimit,
			&r.IsActive, &r.CreatedAt, &r.ParticipantCount); err != nil {
			continue
		}
		rooms = append(rooms, r)
	}

	c.JSON(http.StatusOK, gin.H{"rooms": rooms})
}

// ─────────────────────────────────────────────
// POST /api/v1/voice-rooms/:id/join
// Войти в голосовую комнату
// ─────────────────────────────────────────────

func (h *VoiceRoomHandler) JoinRoom(c *gin.Context) {
	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid room_id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	// Проверяем лимит участников
	var userLimit int
	var participantCount int
	var chatID string
	err = h.sql.QueryRowContext(c, `
		SELECT vr.user_limit, vr.chat_id::text,
		       COUNT(vrp.user_id)
		FROM voice_rooms vr
		LEFT JOIN voice_room_participants vrp ON vrp.room_id = vr.id
		WHERE vr.id = $1 AND vr.is_active = TRUE
		GROUP BY vr.user_limit, vr.chat_id
	`, roomID).Scan(&userLimit, &chatID, &participantCount)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found or inactive"})
		return
	}

	if userLimit > 0 && participantCount >= userLimit {
		c.JSON(http.StatusForbidden, gin.H{"error": "room is full"})
		return
	}

	// Добавляем участника
	_, err = h.sql.ExecContext(c, `
		INSERT INTO voice_room_participants (room_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT (room_id, user_id) DO NOTHING
	`, roomID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to join room"})
		return
	}

	// Уведомляем других участников
	h.ws.publishToChat(chatID, &WSMessage{
		Type:   "voice_room.user_joined",
		From:   userID.String(),
		ChatID: chatID,
		TS:     time.Now().UnixMilli(),
	}, userID.String())

	c.JSON(http.StatusOK, gin.H{"message": "joined", "room_id": roomID})
}

// ─────────────────────────────────────────────
// POST /api/v1/voice-rooms/:id/leave
// Покинуть голосовую комнату
// ─────────────────────────────────────────────

func (h *VoiceRoomHandler) LeaveRoom(c *gin.Context) {
	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid room_id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	var chatID string
	h.sql.QueryRowContext(c, `SELECT chat_id::text FROM voice_rooms WHERE id = $1`, roomID).Scan(&chatID)

	h.sql.ExecContext(c, `
		DELETE FROM voice_room_participants WHERE room_id = $1 AND user_id = $2
	`, roomID, userID)

	// Уведомляем других
	if chatID != "" {
		h.ws.publishToChat(chatID, &WSMessage{
			Type:   "voice_room.user_left",
			From:   userID.String(),
			ChatID: chatID,
			TS:     time.Now().UnixMilli(),
		}, userID.String())
	}

	c.JSON(http.StatusOK, gin.H{"message": "left"})
}

// ─────────────────────────────────────────────
// PUT /api/v1/voice-rooms/:id/state
// Обновить состояние (mute, deafen, video)
// ─────────────────────────────────────────────

type updateStateReq struct {
	IsMuted    *bool `json:"is_muted"`
	IsDeafened *bool `json:"is_deafened"`
	IsVideo    *bool `json:"is_video"`
}

func (h *VoiceRoomHandler) UpdateState(c *gin.Context) {
	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid room_id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	var req updateStateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err = h.sql.ExecContext(c, `
		UPDATE voice_room_participants
		SET is_muted    = COALESCE($3, is_muted),
		    is_deafened = COALESCE($4, is_deafened),
		    is_video    = COALESCE($5, is_video)
		WHERE room_id = $1 AND user_id = $2
	`, roomID, userID, req.IsMuted, req.IsDeafened, req.IsVideo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update state"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "state updated"})
}

// ─────────────────────────────────────────────
// GET /api/v1/voice-rooms/:id/participants
// Список участников комнаты
// ─────────────────────────────────────────────

func (h *VoiceRoomHandler) GetParticipants(c *gin.Context) {
	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid room_id"})
		return
	}

	rows, err := h.sql.QueryContext(c, `
		SELECT vrp.user_id, u.username, COALESCE(u.avatar_url,''),
		       vrp.is_muted, vrp.is_deafened, vrp.is_video, vrp.is_speaking, vrp.joined_at
		FROM voice_room_participants vrp
		JOIN users u ON u.id = vrp.user_id
		WHERE vrp.room_id = $1
		ORDER BY vrp.joined_at ASC
	`, roomID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type participant struct {
		UserID     string    `json:"user_id"`
		Username   string    `json:"username"`
		AvatarURL  string    `json:"avatar_url"`
		IsMuted    bool      `json:"is_muted"`
		IsDeafened bool      `json:"is_deafened"`
		IsVideo    bool      `json:"is_video"`
		IsSpeaking bool      `json:"is_speaking"`
		JoinedAt   time.Time `json:"joined_at"`
	}

	var participants []participant
	for rows.Next() {
		var p participant
		if err := rows.Scan(&p.UserID, &p.Username, &p.AvatarURL,
			&p.IsMuted, &p.IsDeafened, &p.IsVideo, &p.IsSpeaking, &p.JoinedAt); err != nil {
			continue
		}
		participants = append(participants, p)
	}

	c.JSON(http.StatusOK, gin.H{"participants": participants})
}

// ─────────────────────────────────────────────
// POST /api/v1/voice-rooms/:id/raise-hand
// Поднять/опустить руку
// ─────────────────────────────────────────────

func (h *VoiceRoomHandler) RaiseHand(c *gin.Context) {
	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid room_id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	var req struct {
		Raised bool `json:"raised"`
	}
	c.ShouldBindJSON(&req)

	var chatID string
	h.sql.QueryRowContext(c, `SELECT chat_id::text FROM voice_rooms WHERE id = $1`, roomID).Scan(&chatID)

	// Публикуем событие через WebSocket
	if chatID != "" {
		h.ws.publishToChat(chatID, &WSMessage{
			Type:   "voice_room.hand_raised",
			From:   userID.String(),
			ChatID: chatID,
			TS:     time.Now().UnixMilli(),
		}, userID.String())
	}

	c.JSON(http.StatusOK, gin.H{"raised": req.Raised})
}

// ─────────────────────────────────────────────
// DELETE /api/v1/voice-rooms/:id
// Закрыть комнату (только owner/admin)
// ─────────────────────────────────────────────

func (h *VoiceRoomHandler) CloseRoom(c *gin.Context) {
	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid room_id"})
		return
	}

	var chatID string
	h.sql.QueryRowContext(c, `SELECT chat_id::text FROM voice_rooms WHERE id = $1`, roomID).Scan(&chatID)

	h.sql.ExecContext(c, `
		UPDATE voice_rooms SET is_active = FALSE WHERE id = $1
	`, roomID)

	h.sql.ExecContext(c, `
		DELETE FROM voice_room_participants WHERE room_id = $1
	`, roomID)

	if chatID != "" {
		h.ws.publishToChat(chatID, &WSMessage{
			Type:   "voice_room.closed",
			ChatID: chatID,
			TS:     time.Now().UnixMilli(),
		}, "")
	}

	c.JSON(http.StatusOK, gin.H{"message": "room closed"})
}
package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	db "messenger/internal/db/sqlc"
	"messenger/internal/sfu"
)

type VoiceRoomHandler struct {
	q      *db.Queries
	ws     *WSHandler
	sfuCli *sfu.SFUClient
}

func NewVoiceRoomHandler(sqlDB *sql.DB, ws *WSHandler, sfuURL string) *VoiceRoomHandler {
	var sfuCli *sfu.SFUClient
	if sfuURL != "" {
		sfuCli = sfu.NewSFUClient(sfuURL)
	}
	return &VoiceRoomHandler{
		q:      db.New(sqlDB),
		ws:     ws,
		sfuCli: sfuCli,
	}
}

func (h *VoiceRoomHandler) sfuEnabled() bool {
	return h.sfuCli != nil
}

// ── POST /api/v1/chats/:id/voice-rooms ────────────────────────────────────────

type createVoiceRoomReq struct {
	Name      string `json:"name" binding:"required,min=1,max=100"`
	Type      string `json:"type"`
	UserLimit int32  `json:"user_limit"`
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

	room, err := h.q.CreateVoiceRoom(c, db.CreateVoiceRoomParams{
		ID:        uuid.New(),
		ChatID:    chatID,
		Name:      req.Name,
		Type:      sql.NullString{String: roomType, Valid: true},
		UserLimit: sql.NullInt32{Int32: req.UserLimit, Valid: true},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create voice room"})
		return
	}

	h.ws.publishToChat(chatID.String(), &WSMessage{
		Type:   "voice_room.created",
		ChatID: chatID.String(),
		TS:     time.Now().UnixMilli(),
	}, "")

	c.JSON(http.StatusCreated, gin.H{
		"id":          room.ID,
		"chat_id":     room.ChatID,
		"name":        room.Name,
		"type":        room.Type.String,
		"user_limit":  room.UserLimit.Int32,
		"sfu_enabled": h.sfuEnabled(),
	})
}

// ── GET /api/v1/chats/:id/voice-rooms ─────────────────────────────────────────

func (h *VoiceRoomHandler) GetRooms(c *gin.Context) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}

	rooms, err := h.q.GetActiveRoomsByChatID(c, chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type roomRow struct {
		ID               uuid.UUID `json:"id"`
		Name             string    `json:"name"`
		Type             string    `json:"type"`
		UserLimit        int32     `json:"user_limit"`
		IsActive         bool      `json:"is_active"`
		CreatedAt        time.Time `json:"created_at"`
		ParticipantCount int32     `json:"participant_count"`
	}

	result := make([]roomRow, 0, len(rooms))
	for _, r := range rooms {
		result = append(result, roomRow{
			ID:               r.ID,
			Name:             r.Name,
			Type:             r.Type.String,
			UserLimit:        r.UserLimit.Int32,
			IsActive:         r.IsActive.Bool,
			CreatedAt:        r.CreatedAt.Time,
			ParticipantCount: r.ParticipantCount,
		})
	}

	c.JSON(http.StatusOK, gin.H{"rooms": result})
}

// ── POST /api/v1/voice-rooms/:id/join ─────────────────────────────────────────

func (h *VoiceRoomHandler) JoinRoom(c *gin.Context) {
	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid room_id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	row, err := h.q.GetRoomForJoin(c, roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found or inactive"})
		return
	}

	if row.UserLimit.Int32 > 0 && row.ParticipantCount >= row.UserLimit.Int32 {
		c.JSON(http.StatusForbidden, gin.H{"error": "room is full"})
		return
	}

	if err := h.q.JoinVoiceRoom(c, db.JoinVoiceRoomParams{
		RoomID: roomID,
		UserID: userID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to join room"})
		return
	}

	chatID := row.ChatID.String()
	h.ws.publishToChat(chatID, &WSMessage{
		Type:   "voice_room.user_joined",
		From:   userID.String(),
		ChatID: chatID,
		TS:     time.Now().UnixMilli(),
	}, userID.String())

	resp := gin.H{"message": "joined", "room_id": roomID, "sfu_enabled": h.sfuEnabled()}
	if h.sfuEnabled() {
		resp["sfu_url"] = h.sfuCli.BuildConnURL(roomID.String(), userID.String())
	}

	c.JSON(http.StatusOK, resp)
}

// ── POST /api/v1/voice-rooms/:id/leave ────────────────────────────────────────

func (h *VoiceRoomHandler) LeaveRoom(c *gin.Context) {
	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid room_id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	chatID, _ := h.q.GetRoomChatID(c, roomID)

	h.q.LeaveVoiceRoom(c, db.LeaveVoiceRoomParams{
		RoomID: roomID,
		UserID: userID,
	})

	if h.sfuEnabled() {
		h.sfuCli.Disconnect(roomID.String(), userID.String())
	}

	if chatID != uuid.Nil {
		h.ws.publishToChat(chatID.String(), &WSMessage{
			Type:   "voice_room.user_left",
			From:   userID.String(),
			ChatID: chatID.String(),
			TS:     time.Now().UnixMilli(),
		}, userID.String())
	}

	c.JSON(http.StatusOK, gin.H{"message": "left"})
}

// ── PUT /api/v1/voice-rooms/:id/state ─────────────────────────────────────────

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

	params := db.UpdateParticipantStateParams{
		RoomID: roomID,
		UserID: userID,
	}
	if req.IsMuted != nil {
		params.IsMuted = sql.NullBool{Bool: *req.IsMuted, Valid: true}
	}
	if req.IsDeafened != nil {
		params.IsDeafened = sql.NullBool{Bool: *req.IsDeafened, Valid: true}
	}
	if req.IsVideo != nil {
		params.IsVideo = sql.NullBool{Bool: *req.IsVideo, Valid: true}
	}

	if err := h.q.UpdateParticipantState(c, params); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update state"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "state updated"})
}

// ── GET /api/v1/voice-rooms/:id/participants ───────────────────────────────────

func (h *VoiceRoomHandler) GetParticipants(c *gin.Context) {
	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid room_id"})
		return
	}

	rows, err := h.q.GetRoomParticipants(c, roomID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type participant struct {
		UserID     uuid.UUID `json:"user_id"`
		Username   string    `json:"username"`
		AvatarURL  string    `json:"avatar_url"`
		IsMuted    bool      `json:"is_muted"`
		IsDeafened bool      `json:"is_deafened"`
		IsVideo    bool      `json:"is_video"`
		IsSpeaking bool      `json:"is_speaking"`
		JoinedAt   time.Time `json:"joined_at"`
	}

	result := make([]participant, 0, len(rows))
	for _, p := range rows {
		result = append(result, participant{
			UserID:     p.UserID,
			Username:   p.Username,
			AvatarURL:  p.AvatarUrl,
			IsMuted:    p.IsMuted.Bool,
			IsDeafened: p.IsDeafened.Bool,
			IsVideo:    p.IsVideo.Bool,
			IsSpeaking: p.IsSpeaking.Bool,
			JoinedAt:   p.JoinedAt.Time,
		})
	}

	c.JSON(http.StatusOK, gin.H{"participants": result})
}

// ── POST /api/v1/voice-rooms/:id/raise-hand ───────────────────────────────────

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

	chatID, _ := h.q.GetRoomChatID(c, roomID)
	if chatID != uuid.Nil {
		h.ws.publishToChat(chatID.String(), &WSMessage{
			Type:   "voice_room.hand_raised",
			From:   userID.String(),
			ChatID: chatID.String(),
			TS:     time.Now().UnixMilli(),
		}, userID.String())
	}

	c.JSON(http.StatusOK, gin.H{"raised": req.Raised})
}

// ── DELETE /api/v1/voice-rooms/:id ────────────────────────────────────────────

func (h *VoiceRoomHandler) CloseRoom(c *gin.Context) {
	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid room_id"})
		return
	}

	chatID, _ := h.q.GetRoomChatID(c, roomID)
	h.q.CloseVoiceRoom(c, roomID)
	h.q.DeleteRoomParticipants(c, roomID)

	if chatID != uuid.Nil {
		h.ws.publishToChat(chatID.String(), &WSMessage{
			Type:   "voice_room.closed",
			ChatID: chatID.String(),
			TS:     time.Now().UnixMilli(),
		}, "")
	}

	c.JSON(http.StatusOK, gin.H{"message": "room closed"})
}

// ── GET /api/v1/voice-rooms/:id/sfu-capabilities ──────────────────────────────

func (h *VoiceRoomHandler) GetSFUCapabilities(c *gin.Context) {
	if !h.sfuEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SFU not available"})
		return
	}

	roomID := c.Param("id")
	userID := c.MustGet("user_id").(uuid.UUID)

	conn, err := h.sfuCli.Connect(roomID, userID.String())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to connect to SFU"})
		return
	}
	defer conn.Close()

	caps, err := h.sfuCli.GetRouterRtpCapabilities(conn)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to get capabilities"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"rtp_capabilities": caps})
}

// ── POST /api/v1/voice-rooms/:id/sfu-transport ────────────────────────────────

type createTransportReq struct {
	Direction string `json:"direction" binding:"required"`
}

func (h *VoiceRoomHandler) CreateSFUTransport(c *gin.Context) {
	if !h.sfuEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SFU not available"})
		return
	}

	roomID := c.Param("id")
	userID := c.MustGet("user_id").(uuid.UUID)

	var req createTransportReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	conn, err := h.sfuCli.Connect(roomID, userID.String())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "SFU connect failed"})
		return
	}
	defer conn.Close()

	transport, err := h.sfuCli.CreateTransport(conn, req.Direction)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to create transport"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"transport": transport})
}

// ── WebSocket прокси к SFU (/api/v1/voice-rooms/:id/sfu-ws) ───────────────────

var sfuUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (h *VoiceRoomHandler) SFUProxy(c *gin.Context) {
	if !h.sfuEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "SFU not available"})
		return
	}

	roomID := c.Param("id")
	userID := c.MustGet("user_id").(uuid.UUID)

	clientWS, err := sfuUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer clientWS.Close()

	sfuConn, err := h.sfuCli.Connect(roomID, userID.String())
	if err != nil {
		clientWS.WriteMessage(websocket.TextMessage,
			mustJSON(map[string]string{"type": "error", "message": "SFU unavailable"}))
		return
	}
	defer func() {
		sfuConn.Close()
		h.sfuCli.Disconnect(roomID, userID.String())
	}()

	done := make(chan struct{})

	// client → SFU
	go func() {
		defer close(done)
		for {
			mt, msg, err := clientWS.ReadMessage()
			if err != nil {
				return
			}
			if err := sfuConn.WriteMessage(mt, msg); err != nil {
				return
			}
		}
	}()

	// SFU → client
	for {
		select {
		case <-done:
			return
		default:
			mt, msg, err := sfuConn.ReadMessage()
			if err != nil {
				return
			}
			if err := clientWS.WriteMessage(mt, msg); err != nil {
				return
			}
		}
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
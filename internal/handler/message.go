package handler

import (
	"database/sql"
	"encoding/json"
	"messenger/internal/ai"
	db "messenger/internal/db/sqlc"
	"messenger/internal/storage"
	"messenger/internal/store"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type sendMessageReq struct {
	Type        string          `json:"type" binding:"required"`
	Content     string          `json:"content"`
	ReplyToID   *string         `json:"reply_to_id"`
	Format      string          `json:"format"`
	IsSpoiler   bool            `json:"is_spoiler"`
	ScheduledAt *string         `json:"scheduled_at"`
	ExpiresAt   *string         `json:"expires_at"`
	TopicID     *string         `json:"topic_id"`
	MediaID     *string         `json:"media_id"`
	Geo         *GeoPayload     `json:"geo,omitempty"`
	StickerID   *string         `json:"sticker_id,omitempty"`
	Poll        *PollPayload    `json:"poll,omitempty"`
	Contact     *ContactPayload `json:"contact,omitempty"`
	Invoice     *InvoicePayload `json:"invoice,omitempty"`
}
type MessageHandler struct {
	store      *store.MessageStore
	q          *db.Queries
	minio      *storage.MinIOClient
	translator ai.Translator
}

func NewMessageHandler(store *store.MessageStore, sqlDB *sql.DB, minio *storage.MinIOClient, translator ai.Translator) *MessageHandler {
	return &MessageHandler{store: store, q: db.New(sqlDB), minio: minio, translator: translator}
}

//--send

// GeoPayload — геолокация
type GeoPayload struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Title     string  `json:"title,omitempty"`   // название места
	Address   string  `json:"address,omitempty"` // адрес
	IsLive    bool    `json:"is_live,omitempty"` // live location
}

// PollPayload — опрос
type PollPayload struct {
	Question      string   `json:"question" binding:"required"`
	Options       []string `json:"options" binding:"required,min=2,max=10"`
	IsAnonymous   bool     `json:"is_anonymous"`
	IsMultiple    bool     `json:"is_multiple"`              // можно выбрать несколько
	IsQuiz        bool     `json:"is_quiz"`                  // викторина — один правильный ответ
	CorrectOption *int     `json:"correct_option,omitempty"` // для quiz
}

// ContactPayload — контакт
type ContactPayload struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Username  string `json:"username,omitempty"`
	UserID    string `json:"user_id,omitempty"` // если зарегистрирован
}

// InvoicePayload — счёт для оплаты
type InvoicePayload struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Amount      int64  `json:"amount"`            // в копейках/центах
	Currency    string `json:"currency"`          // usd, rub и т.д.
	BotID       string `json:"bot_id"`            // какой бот принимает оплату
	Payload     string `json:"payload,omitempty"` // произвольные данные для бота
}

// @Summary      Send message
// @Description  Send a new message to a chat
// @Tags         messages
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      string         true  "Chat ID"
// @Param        body  body      sendMessageReq true  "Message data"
// @Success      201   {object}  messageResponse
// @Failure      400   {object}  map[string]string
// @Failure      500   {object}  map[string]string
// @Router       /chats/{id}/messages [post]
func (h *MessageHandler) Send(c *gin.Context) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}
	senderID := c.MustGet("user_id").(uuid.UUID)

	// ─── Slow mode check ─────────────────────────────────────────
	slowMode, err := h.q.GetChatSlowMode(c, chatID)
	if err == nil && slowMode.Int32 > 0 {
		lastMsgTime, err := h.q.GetLastMessageTime(c, db.GetLastMessageTimeParams{
			ChatID:   chatID,
			SenderID: senderID,
		})
		if err == nil && !lastMsgTime.IsZero() {
			elapsed := time.Since(lastMsgTime)
			cooldown := time.Duration(slowMode.Int32) * time.Second
			if elapsed < cooldown {
				remaining := int(cooldown.Seconds() - elapsed.Seconds())
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":       "slow mode enabled",
					"retry_after": remaining,
				})
				return
			}
		}
	}

	var req sendMessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	params := db.CreateMessageParams{
		ChatID:   chatID,
		SenderID: senderID,
		Type:     sql.NullString{String: req.Type, Valid: true},
		Content:  req.Content,
	}

	if req.ReplyToID != nil && *req.ReplyToID != "" {
		if id, err := uuid.Parse(*req.ReplyToID); err == nil {
			params.ReplyToID = uuid.NullUUID{UUID: id, Valid: true}
		}
	}
	if req.ScheduledAt != nil && *req.ScheduledAt != "" {
		if t, err := time.Parse(time.RFC3339, *req.ScheduledAt); err == nil {
			params.ScheduledAt = sql.NullTime{Time: t, Valid: true}
		}
	}
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, *req.ExpiresAt); err == nil {
			params.ExpiresAt = sql.NullTime{Time: t, Valid: true}
		}
	}
	if req.MediaID != nil && *req.MediaID != "" {
		if id, err := uuid.Parse(*req.MediaID); err == nil {
			params.MediaID = uuid.NullUUID{UUID: id, Valid: true}
		}
	}

	// ── Типы сообщений ────────────────────────────────────────────
	switch req.Type {
	case "geo":
		if req.Geo == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "geo payload required"})
			return
		}
		geoJSON, err := json.Marshal(req.Geo)
		if err == nil {
			params.Content = string(geoJSON)
		}

	case "sticker":
		if req.StickerID == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sticker_id required"})
			return
		}
		if id, err := uuid.Parse(*req.StickerID); err == nil {
			params.MediaID = uuid.NullUUID{UUID: id, Valid: true}
		}

	case "poll":
		if req.Poll == nil || len(req.Poll.Options) < 2 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "poll requires at least 2 options"})
			return
		}
		pollJSON, err := json.Marshal(req.Poll)
		if err == nil {
			params.Content = string(pollJSON)
		}

	case "contact":
		if req.Contact == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "contact payload required"})
			return
		}
		contactJSON, err := json.Marshal(req.Contact)
		if err == nil {
			params.Content = string(contactJSON)
		}

	case "invoice":
		if req.Invoice == nil || req.Invoice.Amount <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invoice requires positive amount"})
			return
		}
		invoiceJSON, err := json.Marshal(req.Invoice)
		if err == nil {
			params.Content = string(invoiceJSON)
		}
	}

	msg, err := h.store.Send(c, params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send message"})
		return
	}
	c.JSON(http.StatusCreated, toMessageResponse(msg))
}

//--Get

// @Summary      Get chat messages
// @Tags         messages
// @Security     BearerAuth
// @Param        id      path   string  true   "Chat ID"
// @Param        limit   query  int     false  "Limit"
// @Param        offset  query  int     false  "Offset"
// @Success      200  {object}  map[string]any
// @Router       /chats/{id}/messages [get]
func (h *MessageHandler) Getmessages(c *gin.Context) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	limit := int32(50)
	offset := int32(0)
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			limit = int32(v)
		}
	}
	if o := c.Query("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil {
			offset = int32(v)
		}
	}
	messages, err := h.store.GetChatMessages(c, chatID, userID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get messages"})
		return
	}
	result := make([]messageResponse, len(messages))
	for i, m := range messages {
		result[i] = toMessageResponse(m)
	}
	c.JSON(http.StatusOK, gin.H{
		"messages": result,
		"limit":    limit,
		"offset":   offset,
	})
}

// --Edit
type editMessageReq struct {
	Content string `json:"content" binding:"required"`
	Format  string `json:"format"`
}

// @Summary      Edit message
// @Tags         messages
// @Security     BearerAuth
// @Param        id    path  string         true  "Message ID"
// @Param        body  body  editMessageReq true  "New content"
// @Success      200   {object}  messageResponse
// @Failure      403   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Router       /messages/{id} [put]
func (h *MessageHandler) Edit(c *gin.Context) {
	msgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message_id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	var req editMessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	//checking what edits exactly sender
	msg, err := h.store.GetByID(c, msgID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}
	if msg.SenderID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot edit someone else message"})
		return
	}
	updated, err := h.store.Edit(c, msgID, req.Content, sql.NullString{String: req.Format, Valid: req.Format != ""})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to edit message"})
		return
	}
	c.JSON(http.StatusOK, toMessageResponse(

		updated))
}

//--Delete

// @Summary      Delete message for everyone
// @Tags         messages
// @Security     BearerAuth
// @Param        id  path  string  true  "Message ID"
// @Success      200  {object}  map[string]string
// @Failure      403  {object}  map[string]string
// @Router       /messages/{id} [delete]
func (h *MessageHandler) DeleteForAll(c *gin.Context) {
	msgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message_id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	msg, err := h.store.GetByID(c, msgID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}
	if msg.SenderID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot delete someone else message"})
		return
	}

	// Удаляем медиа из MinIO если есть
	if msg.MediaID.Valid {
		media, err := h.q.GetMedia(c, msg.MediaID.UUID)
		if err == nil {
			_ = h.minio.DeleteObject(c, media.ObjectKey)
			if media.ThumbKey.Valid {
				_ = h.minio.DeleteObject(c, media.ThumbKey.String)
			}
			_ = h.q.DeleteMedia(c, media.ID)
		}
	}

	if err := h.store.DeleteForAll(c, msgID, msg.ChatID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete message"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// @Summary      Delete message for me
// @Tags         messages
// @Security     BearerAuth
// @Param        id  path  string  true  "Message ID"
// @Success      200  {object}  map[string]string
// @Router       /messages/{id}/me [delete]
func (h *MessageHandler) DeleteForme(c *gin.Context) {
	msgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message_id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	if err := h.store.DeleteForMe(c, msgID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete message"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted for you"})
}

//--Forward

type forwardReq struct {
	ToChatID uuid.UUID `json:"to_chat_id" binding:"required"`
}

// @Summary      Forward message
// @Tags         messages
// @Security     BearerAuth
// @Param        id    path  string      true  "Message ID"
// @Param        body  body  forwardReq  true  "Target chat"
// @Success      201   {object}  messageResponse
// @Router       /messages/{id}/forward [post]
func (h *MessageHandler) Forward(c *gin.Context) {
	msgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message_id"})
		return
	}
	senderID := c.MustGet("user_id").(uuid.UUID)

	var req forwardReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	msg, err := h.store.Forward(c, msgID, req.ToChatID, senderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to forward message"})
		return
	}
	c.JSON(http.StatusCreated, toMessageResponse(msg))
}

//--Pin

// @Summary      Pin message
// @Tags         messages
// @Security     BearerAuth
// @Param        id  path  string  true  "Message ID"
// @Success      200  {object}  map[string]string
// @Router       /messages/{id}/pin [post]
func (h *MessageHandler) Pin(c *gin.Context) {
	msgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message_id"})
		return
	}
	msg, err := h.store.GetByID(c, msgID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}
	if err := h.store.Pin(c, msgID, msg.ChatID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failded to pin message"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "pinned"})
}

// @Summary      Unpin message
// @Tags         messages
// @Security     BearerAuth
// @Param        id  path  string  true  "Message ID"
// @Success      200  {object}  map[string]string
// @Router       /messages/{id}/pin [delete]
func (h *MessageHandler) Unpin(c *gin.Context) {
	msgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message_id"})
		return
	}
	msg, err := h.store.GetByID(c, msgID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}
	if err := h.store.Unpin(c, msgID, msg.ChatID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failded to pin message"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "unpinned"})
}

// @Summary      Get pinned messages
// @Tags         messages
// @Security     BearerAuth
// @Param        id  path  string  true  "Chat ID"
// @Success      200  {array}  messageResponse
// @Router       /chats/{id}/pinned [get]
func (h *MessageHandler) GetPinned(c *gin.Context) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}
	messages, err := h.store.GetPinned(c, chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failded to get pinned messages"})
		return
	}
	result := make([]messageResponse, len(messages))
	for i, m := range messages {
		result[i] = toMessageResponse(m)
	}
	c.JSON(http.StatusOK, result)
}

// --Reactions
type reactReq struct {
	Emoji string `json:"emoji" binding:"required"`
}

// @Summary      Add reaction
// @Tags         messages
// @Security     BearerAuth
// @Param        id    path  string    true  "Message ID"
// @Param        body  body  reactReq  true  "Emoji"
// @Success      200  {object}  map[string]string
// @Router       /messages/{id}/react [post]
func (h *MessageHandler) AddReaction(c *gin.Context) {
	msgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message_id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	var req reactReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.store.AddReaction(c, msgID, userID, req.Emoji); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failded to add reaction"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "reaction added"})
}

// @Summary      Remove reaction
// @Tags         messages
// @Security     BearerAuth
// @Param        id    path  string    true  "Message ID"
// @Param        body  body  reactReq  true  "Emoji"
// @Success      200  {object}  map[string]string
// @Router       /messages/{id}/react [delete]
func (h *MessageHandler) RemoveReactions(c *gin.Context) {
	msgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message_id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	var req reactReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.store.RemoveReactions(c, msgID, userID, req.Emoji); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failded to remove reaction"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "reaction removed"})
}

//--Read receipts

// @Summary      Mark message as read
// @Tags         messages
// @Security     BearerAuth
// @Param        id  path  string  true  "Message ID"
// @Success      200  {object}  map[string]string
// @Router       /messages/{id}/read [post]
func (h *MessageHandler) MarkRead(c *gin.Context) {
	msgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message_id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	if err := h.store.MarkRead(c, msgID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failded to mark as read"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "marked as read"})
}

// @Summary      Mark chat as read
// @Tags         messages
// @Security     BearerAuth
// @Param        id  path  string  true  "Chat ID"
// @Success      200  {object}  map[string]string
// @Router       /chats/{id}/read [post]
func (h *MessageHandler) MarkChatRead(c *gin.Context) {
	ChatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	if err := h.store.MarkChatRead(c, ChatID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failded to mark chat as read"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "chat marked as read"})
}

// @Summary      Typing indicator
// @Tags         messages
// @Security     BearerAuth
// @Param        id  path  string  true  "Chat ID"
// @Success      200  {object}  map[string]string
// @Router       /chats/{id}/typing [post]
func (h *MessageHandler) Typing(c *gin.Context) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	if err := h.store.SetTyping(c, chatID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failded to set typing"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

//--Saved Messages

// @Summary      Save message
// @Tags         messages
// @Security     BearerAuth
// @Param        id  path  string  true  "Message ID"
// @Success      200  {object}  map[string]string
// @Router       /messages/{id}/save [post]
func (h *MessageHandler) Save(c *gin.Context) {
	msgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message_id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	if err := h.store.Save(c, userID, msgID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failded to set typing"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// @Summary      Get saved messages
// @Tags         messages
// @Security     BearerAuth
// @Param        limit   query  int  false  "Limit"
// @Param        offset  query  int  false  "Offset"
// @Success      200  {array}  messageResponse
// @Router       /messages/saved [get]
func (h *MessageHandler) GetSaved(c *gin.Context) {
	userID := c.MustGet("user_id").(uuid.UUID)

	limit := int32(50)
	offset := int32(0)
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			limit = int32(v)
		}
	}
	if o := c.Query("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil {
			offset = int32(v)
		}
	}
	messages, err := h.store.GetSaved(c, userID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failded to get saved messages"})
		return
	}
	result := make([]messageResponse, len(messages))
	for i, ch := range messages {
		result[i] = toMessageResponse(ch)
	}
	c.JSON(http.StatusOK, result)
}

//--Reminder

type reminderReq struct {
	RemindAt time.Time `json:"remind_at" binding:"required"`
}

// @Summary      Set reminder
// @Tags         messages
// @Security     BearerAuth
// @Param        id    path  string       true  "Message ID"
// @Param        body  body  reminderReq  true  "Reminder time"
// @Success      201  {object}  db.MessageReminder
// @Router       /messages/{id}/reminder [post]
func (h *MessageHandler) SetReminder(c *gin.Context) {
	msgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message_id"})
		return
	}
	userID := c.MustGet("user_id").(uuid.UUID)

	var req reminderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	reminder, err := h.store.SetReminder(c, userID, msgID, req.RemindAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failded to set reminder"})
		return
	}
	c.JSON(http.StatusCreated, reminderResponse{
		ID:        reminder.ID,
		UserID:    reminder.UserID,
		MessageID: reminder.MessageID,
		RemindAt:  reminder.RemindAt,
		IsSent:    reminder.IsSent.Bool,
		CreatedAt: reminder.CreatedAt,
	})
}

//--Search

// @Summary      Search messages
// @Tags         messages
// @Security     BearerAuth
// @Param        id      path   string  true   "Chat ID"
// @Param        q       query  string  true   "Search query"
// @Param        limit   query  int     false  "Limit"
// @Param        offset  query  int     false  "Offset"
// @Success      200  {object}  map[string]any
// @Router       /chats/{id}/messages/search [get]
func (h *MessageHandler) Search(c *gin.Context) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}

	limit := int32(50)
	offset := int32(0)
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			limit = int32(v)
		}
	}
	if o := c.Query("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil {
			offset = int32(v)
		}
	}
	messages, err := h.store.Search(c, chatID, query, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failded to search messages"})
		return
	}
	result := make([]messageResponse, len(messages))
	for i, m := range messages {
		result[i] = toMessageResponse(m)
	}
	c.JSON(http.StatusOK, gin.H{
		"messages": result,
		"query":    query,
	})
}

type messageResponse struct {
	ID              uuid.UUID  `json:"id"`
	ChatID          uuid.UUID  `json:"chat_id"`
	SenderID        uuid.UUID  `json:"sender_id"`
	ReplyToID       *uuid.UUID `json:"reply_to_id,omitempty"`
	ForwardedFromID *uuid.UUID `json:"forwarded_from_id,omitempty"`
	Type            string     `json:"type"`
	Content         string     `json:"content"`
	Format          string     `json:"format,omitempty"`
	IsEdited        bool       `json:"is_edited"`
	IsPinned        bool       `json:"is_pinned"`
	IsSpoiler       bool       `json:"is_spoiler,omitempty"`
	QuotedText      string     `json:"quoted_text,omitempty"`
	ForwardSenderID *uuid.UUID `json:"forward_sender_id,omitempty"`
	ForwardChatID   *uuid.UUID `json:"forward_chat_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at,omitempty"`
	ScheduledAt     *time.Time `json:"scheduled_at,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
}

func toMessageResponse(m db.Message) messageResponse {
	r := messageResponse{
		ID:        m.ID,
		ChatID:    m.ChatID,
		SenderID:  m.SenderID,
		Content:   m.Content,
		IsEdited:  m.IsEdited.Bool,
		IsPinned:  m.IsPinned.Bool,
		IsSpoiler: m.IsSpoiler.Bool,
	}
	if m.Type.Valid {
		r.Type = m.Type.String
	}
	if m.Format.Valid {
		r.Format = m.Format.String
	}
	if m.ReplyToID.Valid {
		r.ReplyToID = &m.ReplyToID.UUID
	}
	if m.ForwardedFromID.Valid {
		r.ForwardedFromID = &m.ForwardedFromID.UUID
	}
	if m.ForwardSenderID.Valid {
		r.ForwardSenderID = &m.ForwardSenderID.UUID
	}
	if m.ForwardChatID.Valid {
		r.ForwardChatID = &m.ForwardChatID.UUID
	}
	if m.QuotedText.Valid {
		r.QuotedText = m.QuotedText.String
	}
	r.CreatedAt = m.CreatedAt
	if m.UpdatedAt.Valid {
		r.UpdatedAt = m.UpdatedAt.Time
	}
	if m.ScheduledAt.Valid {
		r.ScheduledAt = &m.ScheduledAt.Time
	}
	if m.ExpiresAt.Valid {
		r.ExpiresAt = &m.ExpiresAt.Time
	}
	return r
}

type reminderResponse struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	MessageID uuid.UUID `json:"message_id"`
	RemindAt  time.Time `json:"remind_at"`
	IsSent    bool      `json:"is_sent"`
	CreatedAt time.Time `json:"created_at"`
}

// @Summary      Translate message
// @Tags         messages
// @Security     BearerAuth
// @Param        id    path   string  true  "Message ID"
// @Param        lang  query  string  true  "Target language (ru, en, de...)"
// @Success      200  {object}  map[string]string
// @Router       /messages/{id}/translate [get]
func (h *MessageHandler) Translate(c *gin.Context) {
	msgID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message_id"})
		return
	}

	targetLang := c.Query("lang")
	if targetLang == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "lang query param required"})
		return
	}

	msg, err := h.store.GetByID(c, msgID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}

	if msg.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message has no text content"})
		return
	}

	translated, err := h.translator.Translate(c, msg.Content, "", targetLang)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "translation unavailable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message_id":  msgID,
		"original":    msg.Content,
		"translated":  translated,
		"target_lang": targetLang,
	})
}

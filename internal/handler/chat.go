package handler

import (
	"database/sql"
	db "messenger/internal/db/sqlc"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ChatHandler struct {
	q *db.Queries
}

func NewChatHandler(sqlDB *sql.DB) *ChatHandler {
	return &ChatHandler{q: db.New(sqlDB)}
}

//--Create DM

type createDMReq struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
}

// @Summary      Create or get private chat (DM)
// @Tags         chats
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      createDMReq  true  "Target user"
// @Success      200   {object}  chatResponse
// @Failure      400   {object}  map[string]string
// @Router       /chats/dm [post]
func (h *ChatHandler) CreateDM(c *gin.Context) {
	myID := c.MustGet("user_id").(uuid.UUID)

	var req createDMReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if myID == req.UserID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot create chat with yourself"})
		return
	}
	// else chat already exist - return his
	existing, err := h.q.GetPrivateChatBetweenUsers(c, db.GetPrivateChatBetweenUsersParams{
		UserID:   myID,
		UserID_2: req.UserID,
	})
	if err == nil {
		c.JSON(http.StatusOK, toChatResponse(existing))
		return
	}
	// create new chat
	chat, err := h.q.CreateChat(c, db.CreateChatParams{
		Type:    "private",
		OwnerID: uuid.NullUUID{UUID: myID, Valid: true},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create chat "})
		return
	}
	// add both members
	_ = h.q.AddChatMember(c, db.AddChatMemberParams{
		ChatID: chat.ID,
		UserID: myID,
		Role:   sql.NullString{String: "member", Valid: true},
	})
	_ = h.q.AddChatMember(c, db.AddChatMemberParams{
		ChatID: chat.ID,
		UserID: req.UserID,
		Role:   sql.NullString{String: "member", Valid: true},
	})
	c.JSON(http.StatusCreated, toChatResponse(chat))
}

//--Crate Group

type createGroupReq struct {
	Name      string      `json:"name" binding:"required,min=1,max=128"`
	MemberIDs []uuid.UUID `json:"member_ids"`
	IsPublic  bool        `json:"is_public"`
}

// @Summary      Create group chat
// @Tags         chats
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body      createGroupReq  true  "Group data"
// @Success      201   {object}  chatResponse
// @Failure      400   {object}  map[string]string
// @Router       /chats/group [post]
func (h *ChatHandler) CreateGroup(c *gin.Context) {
	myID := c.MustGet("user_id").(uuid.UUID)

	var req createGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	chat, err := h.q.CreateChat(c, db.CreateChatParams{
		Type:     "group",
		Name:     sql.NullString{String: req.Name, Valid: true},
		OwnerID:  uuid.NullUUID{UUID: myID, Valid: true},
		IsPublic: sql.NullBool{Bool: req.IsPublic, Valid: true},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create group"})
		return
	}
	// Owner
	_ = h.q.AddChatMember(c, db.AddChatMemberParams{
		ChatID: chat.ID,
		UserID: myID,
		Role:   sql.NullString{String: "owner", Valid: true},
	})
	// add the rest member
	for _, uid := range req.MemberIDs {
		if uid == myID {
			continue
		}
		_ = h.q.AddChatMember(c, db.AddChatMemberParams{
			ChatID: chat.ID,
			UserID: uid,
			Role:   sql.NullString{String: "member", Valid: true},
		})
	}
	c.JSON(http.StatusCreated, toChatResponse(chat))
}

//--Get my chats

// @Summary      Get my chats
// @Tags         chats
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}  chatResponse
// @Router       /chats [get]
func (h *ChatHandler) GetMyChats(c *gin.Context) {
	myID := c.MustGet("user_id").(uuid.UUID)

	chats, err := h.q.GetUserChats(c, myID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get chats"})
		return
	}
	result := make([]chatResponse, len(chats))
	for i, ch := range chats {
		result[i] = toChatResponse(ch)
	}
	c.JSON(http.StatusOK, result)
}

//--Get chat by ID

// @Summary      Get chat by ID
// @Tags         chats
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  string  true  "Chat ID"
// @Success      200  {object}  chatResponse
// @Failure      404  {object}  map[string]string
// @Router       /chats/{id} [get]
func (h *ChatHandler) GetChat(c *gin.Context) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}
	chat, err := h.q.GetChatByID(c, chatID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "chat not found"})
		return
	}
	c.JSON(http.StatusOK, toChatResponse(chat))
}

//--Get chat members

// @Summary      Get chat members
// @Tags         chats
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  string  true  "Chat ID"
// @Success      200  {array}  memberResponse
// @Router       /chats/{id}/members [get]
func (h *ChatHandler) GetMembers(c *gin.Context) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}
	members, err := h.q.GetChatMembers(c, chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get members"})
		return
	}
	result := make([]memberResponse, len(members))
	for i, m := range members {
		result[i] = toMemberResponse(m)
	}
	c.JSON(http.StatusOK, result)
}

//--Leave chat

// @Summary      Leave chat
// @Tags         chats
// @Security     BearerAuth
// @Param        id  path  string  true  "Chat ID"
// @Success      200  {object}  map[string]string
// @Router       /chats/{id}/leave [post]
func (h *ChatHandler) Leave(c *gin.Context) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}
	myID := c.MustGet("user_id").(uuid.UUID)

	if err := h.q.RemoveChatMember(c, db.RemoveChatMemberParams{
		ChatID: chatID,
		UserID: myID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to leave chat"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "left chat"})
}

type chatResponse struct {
	ID          uuid.UUID  `json:"id"`
	Type        string     `json:"type"`
	Name        string     `json:"name,omitempty"`
	Username    string     `json:"username,omitempty"`
	AvatarURL   string     `json:"avatar_url,omitempty"`
	Description string     `json:"description,omitempty"`
	OwnerID     *uuid.UUID `json:"owner_id,omitempty"`
	IsPublic    bool       `json:"is_public"`
	MemberCount int32      `json:"member_count"`
	InviteLink  string     `json:"invite_link,omitempty"`
	CreatedAt   time.Time  `json:"created_at,omitempty"`
}

func toChatResponse(c db.Chat) chatResponse {
	r := chatResponse{
		ID:          c.ID,
		Type:        c.Type,
		IsPublic:    c.IsPublic.Bool,
		MemberCount: c.MemberCount.Int32,
	}
	if c.Name.Valid {
		r.Name = c.Name.String
	}
	if c.Username.Valid {
		r.Username = c.Username.String
	}
	if c.AvatarUrl.Valid {
		r.AvatarURL = c.AvatarUrl.String
	}
	if c.Description.Valid {
		r.Description = c.Description.String
	}
	if c.OwnerID.Valid {
		r.OwnerID = &c.OwnerID.UUID
	}
	if c.InviteLink.Valid {
		r.InviteLink = c.InviteLink.String
	}
	if c.CreatedAt.Valid {
		r.CreatedAt = c.CreatedAt.Time
	}
	return r
}

type memberResponse struct {
	ChatID   uuid.UUID `json:"chat_id"`
	UserID   uuid.UUID `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at,omitempty"`
}

func toMemberResponse(m db.ChatMember) memberResponse {
	r := memberResponse{
		ChatID: m.ChatID,
		UserID: m.UserID,
	}
	if m.Role.Valid {
		r.Role = m.Role.String
	}
	if m.JoinedAt.Valid {
		r.JoinedAt = m.JoinedAt.Time
	}
	return r
}

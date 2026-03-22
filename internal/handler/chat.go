package handler

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	db "messenger/internal/db/sqlc"
	"net/http"
	"strconv"
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

// ─── Ban / Unban ─────────────────────────────────────────────

type memberActionReq struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
}

type muteReq struct {
	UserID    uuid.UUID `json:"user_id" binding:"required"`
	MuteUntil time.Time `json:"mute_until" binding:"required"` // RFC3339
}

// @Summary Ban member
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Chat ID"
// @Router /chats/{id}/ban [post]
func (h *ChatHandler) BanMember(c *gin.Context) {
	chatID, myID, err := parseChatAndCaller(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.isAdminOrOwner(c, chatID, myID) {
		return
	}
	var req memberActionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.q.BanChatMember(c, db.BanChatMemberParams{
		ChatID: chatID,
		UserID: req.UserID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to ban"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "banned"})
}

// @Summary Unban member
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Chat ID"
// @Router /chats/{id}/unban [post]
func (h *ChatHandler) UnbanMember(c *gin.Context) {
	chatID, myID, err := parseChatAndCaller(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.isAdminOrOwner(c, chatID, myID) {
		return
	}
	var req memberActionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.q.UnbanChatMember(c, db.UnbanChatMemberParams{
		ChatID: chatID,
		UserID: req.UserID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unban"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unbanned"})
}

// @Summary Kick member
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Chat ID"
// @Router /chats/{id}/kick [post]
func (h *ChatHandler) KickMember(c *gin.Context) {
	chatID, myID, err := parseChatAndCaller(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.isAdminOrOwner(c, chatID, myID) {
		return
	}
	var req memberActionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.q.RemoveChatMember(c, db.RemoveChatMemberParams{
		ChatID: chatID,
		UserID: req.UserID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to kick"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "kicked"})
}

// ─── Mute / Unmute ───────────────────────────────────────────

// @Summary Mute member
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Chat ID"
// @Router /chats/{id}/mute [post]
func (h *ChatHandler) MuteMember(c *gin.Context) {
	chatID, myID, err := parseChatAndCaller(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.isAdminOrOwner(c, chatID, myID) {
		return
	}
	var req muteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.q.MuteChatMember(c, db.MuteChatMemberParams{
		ChatID:     chatID,
		UserID:     req.UserID,
		MutedUntil: sql.NullTime{Time: req.MuteUntil, Valid: true},
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mute"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "muted", "until": req.MuteUntil})
}

// @Summary Unmute member
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Chat ID"
// @Router /chats/{id}/unmute [post]
func (h *ChatHandler) UnmuteMember(c *gin.Context) {
	chatID, myID, err := parseChatAndCaller(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.isAdminOrOwner(c, chatID, myID) {
		return
	}
	var req memberActionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.q.UnmuteChatMember(c, db.UnmuteChatMemberParams{
		ChatID: chatID,
		UserID: req.UserID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unmute"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unmuted"})
}

// ─── Helpers ─────────────────────────────────────────────────

func parseChatAndCaller(c *gin.Context) (uuid.UUID, uuid.UUID, error) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid chat_id")
	}
	myID := c.MustGet("user_id").(uuid.UUID)
	return chatID, myID, nil
}

// ─── Role system ─────────────────────────────────────────────

type Role string

const (
	RoleOwner     Role = "owner"
	RoleAdmin     Role = "admin"
	RoleModerator Role = "moderator"
	RoleMember    Role = "member"
	RoleGuest     Role = "guest"
)

// roleLevel — чем выше число тем больше прав
var roleLevel = map[Role]int{
	RoleOwner:     4,
	RoleAdmin:     3,
	RoleModerator: 2,
	RoleMember:    1,
	RoleGuest:     0,
}

func (h *ChatHandler) getMemberRole(c *gin.Context, chatID, userID uuid.UUID) (Role, error) {
	m, err := h.q.GetChatMember(c, db.GetChatMemberParams{
		ChatID: chatID,
		UserID: userID,
	})
	if err != nil {
		return "", err
	}
	return Role(m.Role.String), nil
}

func (h *ChatHandler) hasRole(c *gin.Context, chatID, userID uuid.UUID, min Role) bool {
	role, err := h.getMemberRole(c, chatID, userID)
	if err != nil || roleLevel[role] < roleLevel[min] {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return false
	}
	return true
}

// оставляем для обратной совместимости
func (h *ChatHandler) isAdminOrOwner(c *gin.Context, chatID, myID uuid.UUID) bool {
	return h.hasRole(c, chatID, myID, RoleAdmin)
}

// ─── Invite Links ─────────────────────────────────────────────

type createInviteReq struct {
	MaxUses   int       `json:"max_uses"`   // 0 = unlimited
	ExpiresAt time.Time `json:"expires_at"` // zero = no expiry
}

// @Summary Create invite link
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Chat ID"
// @Router /chats/{id}/invite [post]
func (h *ChatHandler) CreateInvite(c *gin.Context) {
	chatID, myID, err := parseChatAndCaller(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.isAdminOrOwner(c, chatID, myID) {
		return
	}

	var req createInviteReq
	_ = c.ShouldBindJSON(&req)

	code := generateInviteCode()

	var expiresAt sql.NullTime
	if !req.ExpiresAt.IsZero() {
		expiresAt = sql.NullTime{Time: req.ExpiresAt, Valid: true}
	}

	link, err := h.q.CreateInviteLink(c, db.CreateInviteLinkParams{
		Code:      code,
		ChatID:    chatID,
		CreatedBy: myID,
		MaxUses:   sql.NullInt32{Int32: int32(req.MaxUses), Valid: true},
		ExpiresAt: expiresAt,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create invite"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":       link.Code,
		"invite_url": "https://yourdomain.com/join/" + link.Code,
		"expires_at": link.ExpiresAt,
		"max_uses":   link.MaxUses,
	})
}

// @Summary Join chat via invite link
// @Tags chats
// @Security BearerAuth
// @Param code path string true "Invite code"
// @Router /invite/{code} [post]
func (h *ChatHandler) JoinByInvite(c *gin.Context) {
	myID := c.MustGet("user_id").(uuid.UUID)
	code := c.Param("code")

	link, err := h.q.GetInviteByCode(c, code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invite not found or revoked"})
		return
	}

	// Проверяем срок действия
	if link.ExpiresAt.Valid && link.ExpiresAt.Time.Before(time.Now()) {
		c.JSON(http.StatusGone, gin.H{"error": "invite expired"})
		return
	}

	// Проверяем лимит использований
	if link.MaxUses.Valid && link.MaxUses.Int32 > 0 && link.UsesCount.Int32 >= link.MaxUses.Int32 {
		c.JSON(http.StatusGone, gin.H{"error": "invite limit reached"})
		return
	}

	// Добавляем в чат
	_ = h.q.AddChatMember(c, db.AddChatMemberParams{
		ChatID: link.ChatID,
		UserID: myID,
		Role:   sql.NullString{String: "member", Valid: true},
	})

	// Инкрементируем счётчик
	_ = h.q.IncrementInviteUses(c, code)

	c.JSON(http.StatusOK, gin.H{"chat_id": link.ChatID, "status": "joined"})
}

// @Summary Revoke invite link
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Chat ID"
// @Router /chats/{id}/invite/{code} [delete]
func (h *ChatHandler) RevokeInvite(c *gin.Context) {
	chatID, myID, err := parseChatAndCaller(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.isAdminOrOwner(c, chatID, myID) {
		return
	}

	code := c.Param("code")
	if err := h.q.RevokeInviteLink(c, db.RevokeInviteLinkParams{
		Code:   code,
		ChatID: chatID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "revoked"})
}

func generateInviteCode() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// @Summary Set member role
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Chat ID"
// @Router /chats/{id}/role [put]
func (h *ChatHandler) SetRole(c *gin.Context) {
	chatID, myID, err := parseChatAndCaller(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Только owner может менять роли
	if !h.hasRole(c, chatID, myID, RoleOwner) {
		return
	}

	var req struct {
		UserID uuid.UUID `json:"user_id" binding:"required"`
		Role   string    `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Валидация роли
	role := Role(req.Role)
	if _, ok := roleLevel[role]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}
	// owner нельзя назначить через API
	if role == RoleOwner {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot assign owner role"})
		return
	}

	if err := h.q.UpdateChatMemberRole(c, db.UpdateChatMemberRoleParams{
		ChatID: chatID,
		UserID: req.UserID,
		Role:   sql.NullString{String: req.Role, Valid: true},
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set role"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "role": req.Role})
}

// ─── Folders ─────────────────────────────────────────────────

type createFolderReq struct {
	Name     string `json:"name" binding:"required,min=1,max=64"`
	Emoji    string `json:"emoji"`
	Position int32  `json:"position"`
}

// @Summary Create chat folder
// @Tags chats
// @Security BearerAuth
// @Router /folders [post]
func (h *ChatHandler) CreateFolder(c *gin.Context) {
	myID := c.MustGet("user_id").(uuid.UUID)

	var req createFolderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	folder, err := h.q.CreateFolder(c, db.CreateFolderParams{
		UserID:   myID,
		Name:     req.Name,
		Emoji:    sql.NullString{String: req.Emoji, Valid: req.Emoji != ""},
		Position: sql.NullInt32{Int32: req.Position, Valid: true},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create folder"})
		return
	}
	c.JSON(http.StatusCreated, folder)
}

// @Summary Get my folders
// @Tags chats
// @Security BearerAuth
// @Router /folders [get]
func (h *ChatHandler) GetFolders(c *gin.Context) {
	myID := c.MustGet("user_id").(uuid.UUID)

	folders, err := h.q.GetMyFolders(c, myID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get folders"})
		return
	}
	c.JSON(http.StatusOK, folders)
}

// @Summary Delete folder
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Folder ID"
// @Router /folders/{id} [delete]
func (h *ChatHandler) DeleteFolder(c *gin.Context) {
	myID := c.MustGet("user_id").(uuid.UUID)
	folderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid folder_id"})
		return
	}

	if err := h.q.DeleteFolder(c, db.DeleteFolderParams{
		ID:     folderID,
		UserID: myID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete folder"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// @Summary Add chat to folder
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Folder ID"
// @Router /folders/{id}/chats [post]
func (h *ChatHandler) AddToFolder(c *gin.Context) {
	folderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid folder_id"})
		return
	}

	var req struct {
		ChatID uuid.UUID `json:"chat_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.q.AddChatToFolder(c, db.AddChatToFolderParams{
		FolderID: folderID,
		ChatID:   req.ChatID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add chat to folder"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "added"})
}

// @Summary Remove chat from folder
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Folder ID"
// @Param chat_id path string true "Chat ID"
// @Router /folders/{id}/chats/{chat_id} [delete]
func (h *ChatHandler) RemoveFromFolder(c *gin.Context) {
	folderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid folder_id"})
		return
	}
	chatID, err := uuid.Parse(c.Param("chat_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}

	if err := h.q.RemoveChatFromFolder(c, db.RemoveChatFromFolderParams{
		FolderID: folderID,
		ChatID:   chatID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove chat from folder"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}

// @Summary Get chats in folder
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Folder ID"
// @Router /folders/{id}/chats [get]
func (h *ChatHandler) GetFolderChats(c *gin.Context) {
	folderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid folder_id"})
		return
	}

	chats, err := h.q.GetFolderChats(c, folderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get folder chats"})
		return
	}
	result := make([]chatResponse, len(chats))
	for i, ch := range chats {
		result[i] = toChatResponse(ch)
	}
	c.JSON(http.StatusOK, result)
}

// ─── Archive ─────────────────────────────────────────────────

// @Summary Archive chat
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Chat ID"
// @Router /chats/{id}/archive [post]
func (h *ChatHandler) ArchiveChat(c *gin.Context) {
	chatID, myID, err := parseChatAndCaller(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.q.ArchiveChat(c, db.ArchiveChatParams{
		ChatID: chatID,
		UserID: myID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to archive chat"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "archived"})
}

// @Summary Unarchive chat
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Chat ID"
// @Router /chats/{id}/archive [delete]
func (h *ChatHandler) UnarchiveChat(c *gin.Context) {
	chatID, myID, err := parseChatAndCaller(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.q.UnarchiveChat(c, db.UnarchiveChatParams{
		ChatID: chatID,
		UserID: myID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unarchive chat"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unarchived"})
}

// @Summary Get archived chats
// @Tags chats
// @Security BearerAuth
// @Router /chats/archived [get]
func (h *ChatHandler) GetArchived(c *gin.Context) {
	myID := c.MustGet("user_id").(uuid.UUID)

	chats, err := h.q.GetArchivedChats(c, myID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get archived chats"})
		return
	}
	result := make([]chatResponse, len(chats))
	for i, ch := range chats {
		result[i] = toChatResponse(ch)
	}
	c.JSON(http.StatusOK, result)
}

// ─── Channels ────────────────────────────────────────────────

type createChannelReq struct {
	Name        string `json:"name" binding:"required,min=1,max=128"`
	Username    string `json:"username"`
	Description string `json:"description"`
	IsPublic    bool   `json:"is_public"`
}

// @Summary Create channel
// @Tags chats
// @Security BearerAuth
// @Router /chats/channel [post]
func (h *ChatHandler) CreateChannel(c *gin.Context) {
	myID := c.MustGet("user_id").(uuid.UUID)

	var req createChannelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	chat, err := h.q.CreateChannel(c, db.CreateChannelParams{
		Name:        sql.NullString{String: req.Name, Valid: true},
		Username:    sql.NullString{String: req.Username, Valid: req.Username != ""},
		Description: sql.NullString{String: req.Description, Valid: req.Description != ""},
		OwnerID:     uuid.NullUUID{UUID: myID, Valid: true},
		IsPublic:    sql.NullBool{Bool: req.IsPublic, Valid: true},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create channel"})
		return
	}

	// owner автоматически добавляется
	_ = h.q.AddChatMember(c, db.AddChatMemberParams{
		ChatID: chat.ID,
		UserID: myID,
		Role:   sql.NullString{String: "owner", Valid: true},
	})

	c.JSON(http.StatusCreated, toChatResponse(chat))
}

// @Summary Get public chats/channels
// @Tags chats
// @Security BearerAuth
// @Param type query string false "Type: group or channel"
// @Param limit query int false "Limit"
// @Param offset query int false "Offset"
// @Router /chats/public [get]
func (h *ChatHandler) GetPublicChats(c *gin.Context) {
	chatType := c.DefaultQuery("type", "channel")
	limit := int32(20)
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

	chats, err := h.q.GetPublicChats(c, db.GetPublicChatsParams{
		Type:   chatType,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get public chats"})
		return
	}
	result := make([]chatResponse, len(chats))
	for i, ch := range chats {
		result[i] = toChatResponse(ch)
	}
	c.JSON(http.StatusOK, result)
}

// ─── Visibility ───────────────────────────────────────────────

// @Summary Update chat visibility
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Chat ID"
// @Router /chats/{id}/visibility [put]
func (h *ChatHandler) UpdateVisibility(c *gin.Context) {
	chatID, myID, err := parseChatAndCaller(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.hasRole(c, chatID, myID, RoleOwner) {
		return
	}

	var req struct {
		IsPublic bool `json:"is_public"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.q.UpdateChatVisibility(c, db.UpdateChatVisibilityParams{
		ID:       chatID,
		IsPublic: sql.NullBool{Bool: req.IsPublic, Valid: true},
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update visibility"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"is_public": req.IsPublic})
}

// ─── Topics ──────────────────────────────────────────────────

type createTopicReq struct {
	Name      string `json:"name" binding:"required,min=1,max=128"`
	IconEmoji string `json:"icon_emoji"`
	IconColor string `json:"icon_color"`
}

// @Summary Create topic
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Chat ID"
// @Router /chats/{id}/topics [post]
func (h *ChatHandler) CreateTopic(c *gin.Context) {
	chatID, myID, err := parseChatAndCaller(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.isAdminOrOwner(c, chatID, myID) {
		return
	}

	var req createTopicReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	topic, err := h.q.CreateTopic(c, db.CreateTopicParams{
		ChatID:    chatID,
		Name:      req.Name,
		IconEmoji: sql.NullString{String: req.IconEmoji, Valid: req.IconEmoji != ""},
		IconColor: sql.NullString{String: req.IconColor, Valid: req.IconColor != ""},
		CreatedBy: uuid.NullUUID{UUID: myID, Valid: true},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create topic"})
		return
	}
	c.JSON(http.StatusCreated, topic)
}

// @Summary Get chat topics
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Chat ID"
// @Router /chats/{id}/topics [get]
func (h *ChatHandler) GetTopics(c *gin.Context) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}

	topics, err := h.q.GetChatTopics(c, chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get topics"})
		return
	}
	c.JSON(http.StatusOK, topics)
}

// @Summary Close topic
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Chat ID"
// @Param topic_id path string true "Topic ID"
// @Router /chats/{id}/topics/{topic_id}/close [post]
func (h *ChatHandler) CloseTopic(c *gin.Context) {
	chatID, myID, err := parseChatAndCaller(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.isAdminOrOwner(c, chatID, myID) {
		return
	}

	topicID, err := uuid.Parse(c.Param("topic_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid topic_id"})
		return
	}

	if err := h.q.CloseTopic(c, db.CloseTopicParams{
		ID:     topicID,
		ChatID: chatID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to close topic"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "closed"})
}

// @Summary Delete topic
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Chat ID"
// @Param topic_id path string true "Topic ID"
// @Router /chats/{id}/topics/{topic_id} [delete]
func (h *ChatHandler) DeleteTopic(c *gin.Context) {
	chatID, myID, err := parseChatAndCaller(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.isAdminOrOwner(c, chatID, myID) {
		return
	}

	topicID, err := uuid.Parse(c.Param("topic_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid topic_id"})
		return
	}

	if err := h.q.DeleteTopic(c, db.DeleteTopicParams{
		ID:     topicID,
		ChatID: chatID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete topic"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ─── Verification ─────────────────────────────────────────────

// @Summary Verify chat (admin only)
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Chat ID"
// @Router /chats/{id}/verify [post]
func (h *ChatHandler) VerifyChat(c *gin.Context) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}
	if err := h.q.VerifyChat(c, chatID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "verified"})
}

// @Summary Unverify chat (admin only)
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Chat ID"
// @Router /chats/{id}/verify [delete]
func (h *ChatHandler) UnverifyChat(c *gin.Context) {
	chatID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}
	if err := h.q.UnverifyChat(c, chatID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unverify"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unverified"})
}

// ─── Community ───────────────────────────────────────────────

type createCommunityReq struct {
	Name        string `json:"name" binding:"required,min=1,max=128"`
	Username    string `json:"username"`
	Description string `json:"description"`
	IsPublic    bool   `json:"is_public"`
}

// @Summary Create community
// @Tags chats
// @Security BearerAuth
// @Router /chats/community [post]
func (h *ChatHandler) CreateCommunity(c *gin.Context) {
	myID := c.MustGet("user_id").(uuid.UUID)

	var req createCommunityReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	chat, err := h.q.CreateCommunity(c, db.CreateCommunityParams{
		Name:        sql.NullString{String: req.Name, Valid: true},
		Username:    sql.NullString{String: req.Username, Valid: req.Username != ""},
		Description: sql.NullString{String: req.Description, Valid: req.Description != ""},
		OwnerID:     uuid.NullUUID{UUID: myID, Valid: true},
		IsPublic:    sql.NullBool{Bool: req.IsPublic, Valid: true},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create community"})
		return
	}

	_ = h.q.AddChatMember(c, db.AddChatMemberParams{
		ChatID: chat.ID,
		UserID: myID,
		Role:   sql.NullString{String: "owner", Valid: true},
	})

	c.JSON(http.StatusCreated, toChatResponse(chat))
}

// @Summary Add chat to community
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Community ID"
// @Router /chats/{id}/community/chats [post]
func (h *ChatHandler) AddToCommunity(c *gin.Context) {
	communityID, myID, err := parseChatAndCaller(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.isAdminOrOwner(c, communityID, myID) {
		return
	}

	var req struct {
		ChatID uuid.UUID `json:"chat_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.q.AddChatToCommunity(c, db.AddChatToCommunityParams{
		ID:      req.ChatID,
		Column2: communityID.String(),
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add chat to community"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "added"})
}

// @Summary Get community chats
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Community ID"
// @Router /chats/{id}/community/chats [get]
func (h *ChatHandler) GetCommunityChats(c *gin.Context) {
	communityID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid community_id"})
		return
	}

	chats, err := h.q.GetCommunityChats(c, communityID.String())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get community chats"})
		return
	}
	result := make([]chatResponse, len(chats))
	for i, ch := range chats {
		result[i] = toChatResponse(ch)
	}
	c.JSON(http.StatusOK, result)
}

// @Summary Remove chat from community
// @Tags chats
// @Security BearerAuth
// @Param id path string true "Community ID"
// @Param chat_id path string true "Chat ID"
// @Router /chats/{id}/community/chats/{chat_id} [del
func (h *ChatHandler) RemoveFromCommunity(c *gin.Context) {
	communityID, myID, err := parseChatAndCaller(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.isAdminOrOwner(c, communityID, myID) {
		return
	}

	chatID, err := uuid.Parse(c.Param("chat_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}

	if err := h.q.RemoveChatFromCommunity(c, chatID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove chat from community"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}

// ── PUT /api/v1/chats/:id/slow-mode ───────────────────────────────────────────

type setSlowModeReq struct {
	Seconds int32 `json:"seconds" binding:"min=0,max=3600"`
}

func (h *ChatHandler) SetSlowMode(c *gin.Context) {
	chatID, myID, err := parseChatAndCaller(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.isAdminOrOwner(c, chatID, myID) {
		return
	}

	var req setSlowModeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.q.SetSlowMode(c, db.SetSlowModeParams{
		ID:       chatID,
		SlowMode: sql.NullInt32{Int32: req.Seconds, Valid: true},
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set slow mode"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"slow_mode": req.Seconds})
}
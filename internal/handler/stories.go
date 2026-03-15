package handler

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	db "messenger/internal/db/sqlc"
)

// StoriesHandler — Block 8: Stories
type StoriesHandler struct {
	q *db.Queries
}

func NewStoriesHandler(q *db.Queries) *StoriesHandler {
	return &StoriesHandler{q: q}
}

// ── request types ─────────────────────────────────────────────────────────────

type createStoryReq struct {
	MediaURL     string     `json:"media_url"     binding:"required"`
	ThumbnailURL *string    `json:"thumbnail_url"`
	Type         string     `json:"type"`          // image | video (default: image)
	Caption      *string    `json:"caption"`
	Audience     string     `json:"audience"`      // everyone | close_friends | contacts (default: everyone)
	MediaID      *uuid.UUID `json:"media_id"`
	StickerData  []byte     `json:"sticker_data"`
	MusicData    []byte     `json:"music_data"`
}

type reactStoryReq struct {
	Emoji string `json:"emoji" binding:"required"`
}

type addCloseFriendReq struct {
	FriendID uuid.UUID `json:"friend_id" binding:"required"`
}

// ── CreateStory ───────────────────────────────────────────────────────────────

// CreateStory godoc
// @Summary      Создать story
// @Tags         stories
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body createStoryReq true "Story"
// @Success      201 {object} db.Story
// @Router       /stories [post]
func (h *StoriesHandler) CreateStory(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
		return
	}

	var req createStoryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Type == "" {
		req.Type = "image"
	}
	if req.Audience == "" {
		req.Audience = "everyone"
	}

	arg := db.CreateStoryParams{
		UserID:   userID,
		MediaUrl: req.MediaURL,
		Type:     sql.NullString{String: req.Type, Valid: true},
		Audience: sql.NullString{String: req.Audience, Valid: true},
	}
	if req.ThumbnailURL != nil {
		arg.ThumbnailUrl = sql.NullString{String: *req.ThumbnailURL, Valid: true}
	}
	if req.Caption != nil {
		arg.Caption = sql.NullString{String: *req.Caption, Valid: true}
	}
	if req.MediaID != nil {
		arg.MediaID = uuid.NullUUID{UUID: *req.MediaID, Valid: true}
	}
	if len(req.StickerData) > 0 {
		arg.StickerData = pqtype.NullRawMessage{RawMessage: req.StickerData, Valid: true}
	}
	if len(req.MusicData) > 0 {
		arg.MusicData = pqtype.NullRawMessage{RawMessage: req.MusicData, Valid: true}
	}

	story, err := h.q.CreateStory(c.Request.Context(), arg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create story"})
		return
	}
	c.JSON(http.StatusCreated, story)
}

// ── GetFeed ───────────────────────────────────────────────────────────────────

// GetFeed godoc
// @Summary      Лента stories
// @Tags         stories
// @Security     BearerAuth
// @Produce      json
// @Success      200 {array} db.GetStoriesFeedRow
// @Router       /stories/feed [get]
func (h *StoriesHandler) GetFeed(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
		return
	}
	feed, err := h.q.GetStoriesFeed(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get feed"})
		return
	}
	c.JSON(http.StatusOK, feed)
}

// ── GetMyStories ──────────────────────────────────────────────────────────────

// GetMyStories godoc
// @Summary      Мои активные stories
// @Tags         stories
// @Security     BearerAuth
// @Produce      json
// @Success      200 {array} db.GetActiveStoriesRow
// @Router       /stories/my [get]
func (h *StoriesHandler) GetMyStories(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
		return
	}
	stories, err := h.q.GetActiveStories(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stories"})
		return
	}
	c.JSON(http.StatusOK, stories)
}

// ── GetArchived ───────────────────────────────────────────────────────────────

// GetArchived godoc
// @Summary      Архив stories
// @Tags         stories
// @Security     BearerAuth
// @Produce      json
// @Success      200 {array} db.Story
// @Router       /stories/archive [get]
func (h *StoriesHandler) GetArchived(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
		return
	}
	stories, err := h.q.GetArchivedStories(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get archive"})
		return
	}
	c.JSON(http.StatusOK, stories)
}

// ── DeleteStory ───────────────────────────────────────────────────────────────

// DeleteStory godoc
// @Summary      Удалить story
// @Tags         stories
// @Security     BearerAuth
// @Param        id path string true "Story UUID"
// @Success      204
// @Router       /stories/{id} [delete]
func (h *StoriesHandler) DeleteStory(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
		return
	}
	storyID, ok := parseStoryID(c)
	if !ok {
		return
	}
	err := h.q.DeleteStory(c.Request.Context(), db.DeleteStoryParams{
		ID:     storyID,
		UserID: userID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete story"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ── ArchiveStory ──────────────────────────────────────────────────────────────

// ArchiveStory godoc
// @Summary      Переместить story в архив
// @Tags         stories
// @Security     BearerAuth
// @Param        id path string true "Story UUID"
// @Success      204
// @Router       /stories/{id}/archive [post]
func (h *StoriesHandler) ArchiveStory(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
		return
	}
	storyID, ok := parseStoryID(c)
	if !ok {
		return
	}
	err := h.q.ArchiveStory(c.Request.Context(), db.ArchiveStoryParams{
		ID:     storyID,
		UserID: userID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to archive story"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ── ViewStory ─────────────────────────────────────────────────────────────────

// ViewStory godoc
// @Summary      Отметить просмотр story
// @Tags         stories
// @Security     BearerAuth
// @Param        id path string true "Story UUID"
// @Success      204
// @Router       /stories/{id}/view [post]
func (h *StoriesHandler) ViewStory(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
		return
	}
	storyID, ok := parseStoryID(c)
	if !ok {
		return
	}
	_ = h.q.ViewStory(c.Request.Context(), db.ViewStoryParams{
		StoryID: storyID,
		UserID:  userID,
	})
	_ = h.q.IncrementStoryViews(c.Request.Context(), storyID)
	c.Status(http.StatusNoContent)
}

// ── GetViewers ────────────────────────────────────────────────────────────────

// GetViewers godoc
// @Summary      Кто смотрел story (только для автора)
// @Tags         stories
// @Security     BearerAuth
// @Param        id path string true "Story UUID"
// @Success      200 {array} db.GetStoryViewersRow
// @Router       /stories/{id}/viewers [get]
func (h *StoriesHandler) GetViewers(c *gin.Context) {
	storyID, ok := parseStoryID(c)
	if !ok {
		return
	}
	viewers, err := h.q.GetStoryViewers(c.Request.Context(), storyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get viewers"})
		return
	}
	c.JSON(http.StatusOK, viewers)
}

// ── ReactToStory ──────────────────────────────────────────────────────────────

// ReactToStory godoc
// @Summary      Реакция на story
// @Tags         stories
// @Security     BearerAuth
// @Param        id   path string        true "Story UUID"
// @Param        body body reactStoryReq true "Emoji"
// @Success      204
// @Router       /stories/{id}/react [post]
func (h *StoriesHandler) ReactToStory(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
		return
	}
	storyID, ok := parseStoryID(c)
	if !ok {
		return
	}
	var req reactStoryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "emoji required"})
		return
	}
	err := h.q.ReactToStory(c.Request.Context(), db.ReactToStoryParams{
		StoryID: storyID,
		UserID:  userID,
		Emoji:   req.Emoji,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to react"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ── GetReactions ──────────────────────────────────────────────────────────────

// GetReactions godoc
// @Summary      Реакции на story
// @Tags         stories
// @Security     BearerAuth
// @Param        id path string true "Story UUID"
// @Success      200 {array} db.GetStoryReactionsRow
// @Router       /stories/{id}/reactions [get]
func (h *StoriesHandler) GetReactions(c *gin.Context) {
	storyID, ok := parseStoryID(c)
	if !ok {
		return
	}
	reactions, err := h.q.GetStoryReactions(c.Request.Context(), storyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get reactions"})
		return
	}
	c.JSON(http.StatusOK, reactions)
}

// ── Close Friends ─────────────────────────────────────────────────────────────

// GetCloseFriends godoc
// @Summary      Список близких друзей
// @Tags         close-friends
// @Security     BearerAuth
// @Success      200 {array} db.GetCloseFriendsRow
// @Router       /close-friends [get]
func (h *StoriesHandler) GetCloseFriends(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
		return
	}
	friends, err := h.q.GetCloseFriends(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get close friends"})
		return
	}
	c.JSON(http.StatusOK, friends)
}

// AddCloseFriend godoc
// @Summary      Добавить близкого друга
// @Tags         close-friends
// @Security     BearerAuth
// @Param        body body addCloseFriendReq true "Friend ID"
// @Success      204
// @Router       /close-friends [post]
func (h *StoriesHandler) AddCloseFriend(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
		return
	}
	var req addCloseFriendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := h.q.AddCloseFriend(c.Request.Context(), db.AddCloseFriendParams{
		UserID:   userID,
		FriendID: req.FriendID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add close friend"})
		return
	}
	c.Status(http.StatusNoContent)
}

// RemoveCloseFriend godoc
// @Summary      Удалить из близких друзей
// @Tags         close-friends
// @Security     BearerAuth
// @Param        friendID path string true "Friend UUID"
// @Success      204
// @Router       /close-friends/{friendID} [delete]
func (h *StoriesHandler) RemoveCloseFriend(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
		return
	}
	friendID, err := uuid.Parse(c.Param("friendID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid friendID"})
		return
	}
	if err := h.q.RemoveCloseFriend(c.Request.Context(), db.RemoveCloseFriendParams{
		UserID:   userID,
		FriendID: friendID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove close friend"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// mustUserID — достаёт user_id из JWT контекста (уже установлен твоим Auth middleware)
func mustUserID(c *gin.Context) (uuid.UUID, bool) {
	raw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return uuid.Nil, false
	}
	switch v := raw.(type) {
	case uuid.UUID:
		return v, true
	case string:
		id, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user_id"})
			return uuid.Nil, false
		}
		return id, true
	}
	c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
	return uuid.Nil, false
}

func parseStoryID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid story id"})
		return uuid.Nil, false
	}
	return id, true
}

// Sentinel для обратной совместимости
var _ = errors.New
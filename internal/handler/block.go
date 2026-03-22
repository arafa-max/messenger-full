package handler

import (
	"net/http"

	db "messenger/internal/db/sqlc"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type BlockHandler struct {
	queries *db.Queries
}

func NewBlockHandler(q *db.Queries) *BlockHandler {
	return &BlockHandler{queries: q}
}

// getUserID — читает UUID из контекста Gin (middleware кладёт его как uuid.UUID, не string)
func getUserID(c *gin.Context) (uuid.UUID, bool) {
	val, exists := c.Get("user_id")
	if !exists {
		return uuid.Nil, false
	}
	id, ok := val.(uuid.UUID)
	return id, ok
}

func (h *BlockHandler) BlockUser(c *gin.Context) {
	blockerID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	blockedID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_user_id"})
		return
	}

	if blockerID == blockedID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot_block_yourself"})
		return
	}

	err = h.queries.BlockUser(c.Request.Context(), db.BlockUserParams{
		BlockerID: blockerID,
		BlockedID: blockedID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user_blocked"})
}

func (h *BlockHandler) UnblockUser(c *gin.Context) {
	blockerID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	blockedID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_user_id"})
		return
	}

	err = h.queries.UnblockUser(c.Request.Context(), db.UnblockUserParams{
		BlockerID: blockerID,
		BlockedID: blockedID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user_unblocked"})
}

func (h *BlockHandler) GetBlockedUsers(c *gin.Context) {
	blockerID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	users, err := h.queries.GetBlockedUsers(c.Request.Context(), blockerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	c.JSON(http.StatusOK, users)
}
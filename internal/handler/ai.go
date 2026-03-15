package handler

import (
	"net/http"

	"messenger/internal/ai"
	db "messenger/internal/db/sqlc"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AIHandler struct {
	ai      ai.LLMClient
	queries *db.Queries
}

func NewAIHandler(aiClient ai.LLMClient, queries *db.Queries) *AIHandler {
	return &AIHandler{
		ai:      aiClient,
		queries: queries,
	}
}

func (h *AIHandler) SmartReply(c *gin.Context) {
	chatIDStr := c.Param("id") // у тебя /:id в роутах
	userIDStr := c.GetString("user_id")

	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"replies": []string{"Окей", "Понял", "Позже"}})
		return
	}

	msgs, err := h.queries.GetChatMessages(c, db.GetChatMessagesParams{
		ChatID: chatID,
		Limit:  10,
		Offset: 0,
		UserID: userID,
	})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"replies": []string{"Окей", "Понял", "Позже"}})
		return
	}

	aiMsgs := make([]ai.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Content != "" {
			aiMsgs = append(aiMsgs, ai.Message{Text: m.Content})
		}
	}

	replies, err := h.ai.SmartReply(c, aiMsgs)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"replies": []string{"Окей", "Понял", "Позже"}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"replies": replies})
}

func (h *AIHandler) ChatSummary(c *gin.Context) {
	chatIDStr := c.Param("id")
	userIDStr := c.GetString("user_id")

	chatID, err := uuid.Parse(chatIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid chat_id"})
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"summary": "Резюме недоступно."})
		return
	}

	msgs, err := h.queries.GetChatMessages(c, db.GetChatMessagesParams{
		ChatID: chatID,
		Limit:  50,
		Offset: 0,
		UserID: userID,
	})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"summary": "Резюме недоступно."})
		return
	}

	aiMsgs := make([]ai.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Content != "" {
			aiMsgs = append(aiMsgs, ai.Message{Text: m.Content})
		}
	}

	summary, err := h.ai.Summarize(c, aiMsgs)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"summary": "Резюме временно недоступно."})
		return
	}

	c.JSON(http.StatusOK, gin.H{"summary": summary})
}
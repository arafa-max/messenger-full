package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"messenger/internal/bot"
	db "messenger/internal/db/sqlc"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type BotHandler struct {
	queries    *db.Queries
	dispatcher *bot.Dispatcher
}

func NewBotHandler(queries *db.Queries, dispatcher *bot.Dispatcher) *BotHandler {
	return &BotHandler{
		queries:    queries,
		dispatcher: dispatcher,
	}
}

// POST /bots — создать бота
func (h *BotHandler) CreateBot(c *gin.Context) {
	ownerID, ok := getUserID(c) // FIX: используем getUserID вместо GetString
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req struct {
		Username    string `json:"username"     binding:"required"`
		Name        string `json:"name"         binding:"required"`
		Description string `json:"description"`
		IsAIEnabled bool   `json:"is_ai_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token := "bot_" + uuid.New().String()

	bot, err := h.queries.CreateBot(c, db.CreateBotParams{
		OwnerID:     ownerID,
		Token:       token,
		Username:    req.Username,
		Name:        req.Name,
		Description: req.Description,
		IsAiEnabled: req.IsAIEnabled,
	})
	if err != nil {
		log.Printf("❌ CreateBot error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create bot"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"bot":   bot,
		"token": token,
	})
}

// GET /bots — мои боты
func (h *BotHandler) GetMyBots(c *gin.Context) {
	ownerID, ok := getUserID(c) // FIX
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	bots, err := h.queries.GetMyBots(c, ownerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get bots"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"bots": bots})
}

// DELETE /bots/:id — деактивировать бота
func (h *BotHandler) DeactivateBot(c *gin.Context) {
	ownerID, ok := getUserID(c) // FIX
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	botID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bot_id"})
		return
	}

	err = h.queries.DeactivateBot(c, db.DeactivateBotParams{
		ID:      botID,
		OwnerID: ownerID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to deactivate bot"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// PUT /bots/:id/webhook — установить webhook URL
func (h *BotHandler) SetWebhook(c *gin.Context) {
	botID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bot_id"})
		return
	}

	var req struct {
		URL    string `json:"url"    binding:"required"`
		Secret string `json:"secret"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = h.queries.UpdateBotWebhook(c, db.UpdateBotWebhookParams{
		ID:         botID,
		WebhookUrl: req.URL,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set webhook"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /bot/webhook/:token — входящий update от клиента
func (h *BotHandler) HandleWebhook(c *gin.Context) {
	token := c.Param("token")

	bot, err := h.queries.GetBotByToken(c, token)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	if bot.WebhookSecret != "" {
		secret := c.GetHeader("X-Bot-Secret")
		if secret != bot.WebhookSecret {
			c.Status(http.StatusUnauthorized)
			return
		}
	}

	var update struct {
		UpdateID int64           `json:"update_id"`
		Type     string          `json:"type"`
		Payload  json.RawMessage `json:"payload"`
	}
	if err := c.ShouldBindJSON(&update); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	_ = h.queries.SaveBotUpdate(c, db.SaveBotUpdateParams{
		BotID:    bot.ID,
		UpdateID: update.UpdateID,
		Type:     update.Type,
		Payload:  update.Payload,
	})
	go h.dispatcher.Dispatch(context.Background(), bot, update.Payload)

	c.Status(http.StatusOK)
}

// GET /bots/:id/commands — список команд бота
func (h *BotHandler) GetCommands(c *gin.Context) {
	botID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bot_id"})
		return
	}

	commands, err := h.queries.GetBotCommands(c, botID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get commands"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"commands": commands})
}

// POST /bots/:id/commands — добавить команду
func (h *BotHandler) AddCommand(c *gin.Context) {
	botID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bot_id"})
		return
	}

	var req struct {
		Command     string `json:"command"     binding:"required"`
		Description string `json:"description" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = h.queries.CreateBotCommand(c, db.CreateBotCommandParams{
		BotID:       botID,
		Command:     req.Command,
		Description: req.Description,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add command"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"ok": true})
}

package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"messenger/internal/ai"

	"github.com/gin-gonic/gin"
)

type ModerationMiddleware struct {
	ai ai.LLMClient
}

func NewModeration(aiClient ai.LLMClient) *ModerationMiddleware {
	return &ModerationMiddleware{ai: aiClient}
}

func (m *ModerationMiddleware) Handle() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Читаем body
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Next()
			return
		}
		// Восстанавливаем body для следующего handler
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Достаём content из JSON
		var body struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(bodyBytes, &body); err != nil || body.Content == "" {
			c.Next()
			return
		}

		// Проверяем модерацию
		result, err := m.ai.Moderate(c.Request.Context(), body.Content)
		if err != nil {
			// Ошибка модерации — не блокируем
			c.Next()
			return
		}

		if result.IsSpam || result.IsToxic {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":    "message blocked by moderation",
				"is_spam":  result.IsSpam,
				"is_toxic": result.IsToxic,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

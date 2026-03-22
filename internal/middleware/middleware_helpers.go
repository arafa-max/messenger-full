package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// getUserIDFromContext — получает UUID из Gin контекста.
// Middleware кладёт user_id как uuid.UUID объект (не строку),
// поэтому GetString не работает — нужен type assertion.
func getUserIDFromContext(c *gin.Context) (uuid.UUID, bool) {
	val, exists := c.Get("user_id")
	if !exists {
		return uuid.Nil, false
	}
	id, ok := val.(uuid.UUID)
	return id, ok
}
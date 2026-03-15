package middleware

import (
	"net/http"

	db "messenger/internal/db/sqlc"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequirePremium — middleware для Premium-только роутов
func RequirePremium(queries *db.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := uuid.Parse(c.GetString("user_id"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		sub, err := queries.GetSubscriptionByUserID(c, userID)
		if err != nil || sub.Status != "active" {
			c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
				"error": "premium subscription required",
				"code":  "premium_required",
			})
			return
		}

		// Пробрасываем план в контекст
		c.Set("subscription_plan", sub.Plan)
		c.Next()
	}
}

// InjectPremiumStatus — не блокирует, просто добавляет статус в контекст
func InjectPremiumStatus(queries *db.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := uuid.Parse(c.GetString("user_id"))
		if err != nil {
			c.Set("is_premium", false)
			c.Next()
			return
		}

		sub, err := queries.GetSubscriptionByUserID(c, userID)
		isPremium := err == nil && sub.Status == "active"
		c.Set("is_premium", isPremium)
		c.Set("subscription_plan", func() string {
			if isPremium {
				return sub.Plan
			}
			return "free"
		}())
		c.Next()
	}
}
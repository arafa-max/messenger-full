package middleware

import (
	"net/http"

	db "messenger/internal/db/sqlc"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func BanCheck(queries *db.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDStr := c.GetString("userID")
		if userIDStr == "" {
			c.Next()
			return
		}

		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			c.Next()
			return
		}

		banned, err := queries.IsUserBanned(c.Request.Context(), userID)
		if err != nil {
			c.Next()
			return
		}

		if banned {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "account_banned",
				"message": "Your account has been banned for violating Terms of Service",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
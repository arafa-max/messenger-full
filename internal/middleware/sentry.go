package middleware

import (
	"fmt"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
)

// Sentry middleware — ловит паники и ошибки, отправляет в Sentry.
// Ставить ПЕРВЫМ в цепочке middleware (до Recovery).
func Sentry() gin.HandlerFunc {
	return sentrygin.New(sentrygin.Options{
		Repanic: true, // передаёт панику дальше → gin.Recovery её поймает
	})
}

// SentryUser добавляет информацию о пользователе в Sentry событие.
// Ставить ПОСЛЕ middleware.Auth — когда userID уже известен.
func SentryUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("userID")
		if !exists {
			c.Next()
			return
		}

		if hub := sentrygin.GetHubFromContext(c); hub != nil {
			hub.Scope().SetUser(sentry.User{
				ID: fmt.Sprintf("%v", userID),
			})
		}

		c.Next()
	}
}
package middleware

import (
	"context"
	"log"
	"messenger/internal/auth"
	"messenger/internal/config"
	rdb "messenger/internal/redis"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func Auth(cfg *config.Config, redisClient ...*rdb.Client) gin.HandlerFunc {
	jwtMgr := auth.NewJWTManager(
		cfg.JWT.AccessSecret,
		cfg.JWT.RefreshSecret,
		cfg.JWT.AccessMinutes,
		cfg.JWT.RefreshDays,
	)
	return func(c *gin.Context) {
		var tokenStr string

		header := c.GetHeader("Authorization")
		if header != "" && strings.HasPrefix(header, "Bearer ") {
			tokenStr = strings.TrimPrefix(header, "Bearer ")
		} else if q := c.Query("token"); q != "" {
			// fallback для WebSocket (браузер не может передать заголовки)
			tokenStr = q
		} else {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}

		claims, err := jwtMgr.ParseAccess(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("jti", claims.JTI)

		if len(redisClient) > 0 && redisClient[0] != nil {
			log.Println("Setting online for", claims.UserID.String())
			go redisClient[0].SetOnline(context.Background(), claims.UserID.String())
		}
		c.Next()
	}
}

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type,X-Request-ID")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = uuid.New().String()
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)

		status := c.Writer.Status()
		method := c.Request.Method
		path := c.Request.URL.Path

		var color string
		switch {
		case status >= 500:
			color = "\033[31m"
		case status >= 400:
			color = "\033[33m"
		default:
			color = "\033[32m"

		}
		reset := "\033[0m"

		_ = color
		_ = reset

		gin.DefaultWriter.Write([]byte(
			"[GIN] " + method + " " + path +
				" | " + http.StatusText(status) +
				" | " + latency.String() + "\n",
		))
	}
}

func GetUserID(c *gin.Context) uuid.UUID {
	val, _ := c.Get("user_id")
	id, _ := val.(uuid.UUID)
	return id
}

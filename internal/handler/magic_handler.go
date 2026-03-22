package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"messenger/internal/auth"
	"messenger/internal/config"
	db "messenger/internal/db/sqlc"
	"messenger/internal/email"
	"messenger/internal/redis"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	magicLinkPrefix = "magic:"
	magicLinkTTL    = 15 * time.Minute
)

type MagicLinkHandler struct {
	q      *db.Queries
	jwt    *auth.JWTManager
	cfg    *config.Config
	rdb    *redis.Client
	mailer *email.Client
}

func NewMagicLinkHandler(sqlDB interface{ QueryContext(...interface{}) error }, cfg *config.Config, rdb *redis.Client, mailer *email.Client) *MagicLinkHandler {
	return nil // placeholder — см. ниже правильный конструктор
}

// используй этот:
func NewMagicHandler(queries *db.Queries, cfg *config.Config, rdb *redis.Client, mailer *email.Client) *MagicLinkHandler {
	return &MagicLinkHandler{
		q:      queries,
		jwt:    auth.NewJWTManager(cfg.JWT.AccessSecret, cfg.JWT.RefreshSecret, cfg.JWT.AccessMinutes, cfg.JWT.RefreshDays),
		cfg:    cfg,
		rdb:    rdb,
		mailer: mailer,
	}
}

// POST /api/v1/auth/magic/request
// Отправляет magic link на email
func (h *MagicLinkHandler) Request(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid email required"})
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	// Rate limit — не более 3 запросов в 10 минут с одного email
	ratKey := fmt.Sprintf("magic:rate:%s", req.Email)
	count, err := h.rdb.IncrExpire(c, ratKey, 10*time.Minute)
	if err == nil && count > 3 {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many requests, try again later"})
		return
	}

	// Генерируем токен
	token, err := generateMagicToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	// Сохраняем email → token в Redis
	if err := h.rdb.SetEx(c, magicLinkPrefix+token, req.Email, magicLinkTTL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save token"})
		return
	}

	// Строим ссылку
	magicURL := fmt.Sprintf("%s/api/v1/auth/magic/verify?token=%s",
		h.cfg.Server.PublicURL, token)

	// Отправляем письмо
	if err := h.mailer.SendMagicLink(c.Request.Context(), req.Email, magicURL); err != nil {
		// Удаляем токен если письмо не отправилось
		h.rdb.Delete(c, magicLinkPrefix+token)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send email"})
		return
	}

	// Всегда отвечаем одинаково — не раскрываем существует ли email
	c.JSON(http.StatusOK, gin.H{
		"message": "if this email is registered, you will receive a login link",
	})
}

// GET /api/v1/auth/magic/verify?token=xxx
// Верифицирует токен и выдаёт JWT
func (h *MagicLinkHandler) Verify(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token required"})
		return
	}

	// Достаём email из Redis
	emailAddr, err := h.rdb.Get(c, magicLinkPrefix+token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired token"})
		return
	}

	// One-time use — удаляем сразу
	h.rdb.Delete(c, magicLinkPrefix+token)

	// Ищем пользователя по email
	user, err := h.q.GetUserByEmail(c, sql.NullString{String: emailAddr, Valid: true})
	if err != nil {
		// Пользователя нет — создаём автоматически
		username := generateUsernameFromEmail(emailAddr)
		user, err = h.q.CreateUser(c, db.CreateUserParams{
			Username: username,
			Email:    sql.NullString{String: emailAddr, Valid: true},
			Password: "",
			Language: sql.NullString{String: "ru", Valid: true},
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
			return
		}
	}

	// Выдаём токены
	access, err := h.jwt.GenerateAccess(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue token"})
		return
	}
	refresh, err := h.jwt.GenerateRefresh(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue token"})
		return
	}

	_, err = h.q.CreateSession(c, db.CreateSessionParams{
		UserID:       user.ID,
		RefreshToken: refresh,
		IpAddress:    parseInet(c.ClientIP()),
		ExpiresAt:    time.Now().AddDate(0, 0, h.cfg.JWT.RefreshDays),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": toSafeUser(user),
		"tokens": tokenPair{
			AccessToken:  access,
			RefreshToken: refresh,
			ExpiresIn:    h.cfg.JWT.AccessMinutes * 60,
		},
	})
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func generateMagicToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func generateUsernameFromEmail(emailAddr string) string {
	parts := strings.Split(emailAddr, "@")
	base := strings.ToLower(parts[0])
	// убираем спецсимволы
	base = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, base)
	suffix := make([]byte, 3)
	rand.Read(suffix)
	return fmt.Sprintf("%s_%s", base, hex.EncodeToString(suffix))
}
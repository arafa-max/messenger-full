package middleware

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	rdb "messenger/internal/redis"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// SpamConfig — настройки антиспама, можно переопределить через .env
type SpamConfig struct {
	// Flood
	FloodMaxMessages int           // макс сообщений за окно (default: 5)
	FloodWindow      time.Duration // размер окна (default: 5s)

	// Дубли
	DuplicateWindow time.Duration // окно для проверки дублей (default: 30s)

	// Контентные эвристики
	MaxURLs       int     // макс ссылок в сообщении (default: 3)
	MaxCapsRatio  float64 // макс доля заглавных букв (default: 0.7)
	MaxRepeatRune int     // макс повторов одного символа подряд (default: 8)
	MinMsgLen     int     // мин длина сообщения (default: 1)
	MaxMsgLen     int     // макс длина (default: 4096)

	// Новый участник
	NewMemberWindow    time.Duration // окно ограничений для новых (default: 5min)
	NewMemberMaxMsgs   int           // макс сообщений для новых (default: 3)
}

func DefaultSpamConfig() SpamConfig {
	return SpamConfig{
		FloodMaxMessages:   5,
		FloodWindow:        5 * time.Second,
		DuplicateWindow:    30 * time.Second,
		MaxURLs:            3,
		MaxCapsRatio:       0.7,
		MaxRepeatRune:      8,
		MinMsgLen:          1,
		MaxMsgLen:          4096,
		NewMemberWindow:    5 * time.Minute,
		NewMemberMaxMsgs:   3,
	}
}

type AntiSpam struct {
	rdb *rdb.Client
	cfg SpamConfig
}

func NewAntiSpam(rdb *rdb.Client, cfg ...SpamConfig) *AntiSpam {
	c := DefaultSpamConfig()
	if len(cfg) > 0 {
		c = cfg[0]
	}
	return &AntiSpam{rdb: rdb, cfg: c}
}

// Handle — основной middleware, вешается на POST /chats/:id/messages
func (a *AntiSpam) Handle() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			c.Next()
			return
		}
		uid := userID.(uuid.UUID)
		chatID := c.Param("id")

		// Читаем тело — нам нужен content
		// Используем уже забинденный контент если есть,
		// или берём из запроса через peek
		content := peekMessageContent(c)

		ctx := c.Request.Context()

		// ── 1. Flood check ───────────────────────────────────────
		if blocked, reason := a.checkFlood(ctx, uid, chatID); blocked {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": reason,
				"retry_after": a.cfg.FloodWindow.Seconds(),
			})
			return
		}

		// ── 2. Duplicate check ───────────────────────────────────
		if content != "" {
			if blocked, reason := a.checkDuplicate(ctx, uid, chatID, content); blocked {
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"error": reason,
				})
				return
			}
		}

		// ── 3. Content heuristics ────────────────────────────────
		if content != "" {
			if blocked, reason := a.checkContent(content); blocked {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"error": reason,
				})
				return
			}
		}

		// ── 4. New member throttle ───────────────────────────────
		if blocked, reason := a.checkNewMember(ctx, uid, chatID); blocked {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": reason,
				"retry_after": a.cfg.NewMemberWindow.Seconds(),
			})
			return
		}

		c.Next()
	}
}

// ─── Flood ────────────────────────────────────────────────────────────────────

func (a *AntiSpam) checkFlood(ctx context.Context, userID uuid.UUID, chatID string) (bool, string) {
	key := fmt.Sprintf("spam:flood:%s:%s", userID, chatID)

	count, err := a.rdb.IncrExpire(ctx, key, a.cfg.FloodWindow)
	if err != nil {
		return false, "" // при ошибке Redis — пропускаем
	}

	if count > int64(a.cfg.FloodMaxMessages) {
		return true, fmt.Sprintf("too many messages, slow down (max %d per %s)",
			a.cfg.FloodMaxMessages, a.cfg.FloodWindow)
	}
	return false, ""
}

// ─── Duplicate ────────────────────────────────────────────────────────────────

func (a *AntiSpam) checkDuplicate(ctx context.Context, userID uuid.UUID, chatID, content string) (bool, string) {
	// Хэшируем контент чтобы не хранить текст в Redis
	key := fmt.Sprintf("spam:dup:%s:%s:%d", userID, chatID, simpleHash(content))

	exists, err := a.rdb.Exists(ctx, key)
	if err != nil {
		return false, ""
	}
	if exists {
		return true, "duplicate message"
	}

	// Сохраняем на время окна
	_ = a.rdb.SetEx(ctx, key, "1", a.cfg.DuplicateWindow)
	return false, ""
}

// ─── Content heuristics ───────────────────────────────────────────────────────

func (a *AntiSpam) checkContent(content string) (bool, string) {
	// Длина
	if len([]rune(content)) < a.cfg.MinMsgLen {
		return true, "message too short"
	}
	if len([]rune(content)) > a.cfg.MaxMsgLen {
		return true, fmt.Sprintf("message too long (max %d chars)", a.cfg.MaxMsgLen)
	}

	// Слишком много ссылок
	if countURLs(content) > a.cfg.MaxURLs {
		return true, fmt.Sprintf("too many links (max %d)", a.cfg.MaxURLs)
	}

	// Caps lock
	if capsRatio(content) > a.cfg.MaxCapsRatio {
		return true, "too many capital letters"
	}

	// Повторяющиеся символы: аааааааа
	if maxRepeatRune(content) > a.cfg.MaxRepeatRune {
		return true, "repeated characters detected"
	}

	return false, ""
}

// ─── New member throttle ──────────────────────────────────────────────────────

func (a *AntiSpam) checkNewMember(ctx context.Context, userID uuid.UUID, chatID string) (bool, string) {
	key := fmt.Sprintf("spam:newmember:%s:%s", userID, chatID)

	count, err := a.rdb.IncrExpire(ctx, key, a.cfg.NewMemberWindow)
	if err != nil {
		return false, ""
	}

	// Первое сообщение — устанавливаем TTL
	// count==1 значит ключ только что создан — это новый участник в окне
	if count == 1 {
		// Новый участник — начинаем отсчёт
		return false, ""
	}

	if count > int64(a.cfg.NewMemberMaxMsgs) {
		// Проверяем что ключ действительно свежий (участник новый)
		ttl, err := a.rdb.TTL(ctx, key)
		if err == nil && ttl > 0 && ttl <= a.cfg.NewMemberWindow {
			remaining := a.cfg.NewMemberWindow - ttl
			if remaining < a.cfg.NewMemberWindow/2 {
				return true, "new members are limited in messages, please wait"
			}
		}
	}

	return false, ""
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func peekMessageContent(c *gin.Context) string {
	// Пробуем достать из уже установленного контекста (если moderation уже читал)
	if v, exists := c.Get("message_content"); exists {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func countURLs(text string) int {
	count := 0
	words := strings.Fields(text)
	for _, w := range words {
		if strings.HasPrefix(w, "http://") || strings.HasPrefix(w, "https://") {
			if _, err := url.ParseRequestURI(w); err == nil {
				count++
			}
		}
	}
	return count
}

func capsRatio(text string) float64 {
	var letters, upper int
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	if letters < 5 {
		return 0 // слишком короткий — не считаем
	}
	return math.Round(float64(upper)/float64(letters)*100) / 100
}

func maxRepeatRune(text string) int {
	if len(text) == 0 {
		return 0
	}
	runes := []rune(text)
	max, cur := 1, 1
	for i := 1; i < len(runes); i++ {
		if runes[i] == runes[i-1] {
			cur++
			if cur > max {
				max = cur
			}
		} else {
			cur = 1
		}
	}
	return max
}

// simpleHash — быстрый некриптографический хэш для дедупликации
func simpleHash(s string) uint32 {
	var h uint32 = 2166136261
	for _, c := range []byte(s) {
		h ^= uint32(c)
		h *= 16777619
	}
	return h
}
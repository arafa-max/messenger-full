package handler

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// TURNConfig — конфиг TURN сервера (из env/config)
type TURNConfig struct {
	Host       string // например: turn.yourdomain.com
	Port       int    // 3478
	TLSPort    int    // 5349
	AuthSecret string // static-auth-secret из turnserver.conf
	TTL        int    // время жизни credentials в секундах (по умолчанию 86400 = 24ч)
}

// TURNCredentials — временные credentials для клиента
type TURNCredentials struct {
	Username   string   `json:"username"`    // timestamp:userID
	Password   string   `json:"password"`    // HMAC-SHA1
	TTL        int      `json:"ttl"`         // секунды до истечения
	URIs       []string `json:"uris"`        // список TURN/STUN серверов
}

// TURNHandler — хендлер для TURN credentials
type TURNHandler struct {
	cfg TURNConfig
}

func NewTURNHandler(cfg TURNConfig) *TURNHandler {
	if cfg.TTL == 0 {
		cfg.TTL = 86400 // 24 часа по умолчанию
	}
	return &TURNHandler{cfg: cfg}
}

// GetTURNCredentials — GET /api/v1/calls/turn
// @Summary Получить временные TURN credentials
// @Description Клиент вызывает перед каждым звонком. Credentials действуют 24ч.
// @Tags Calls
// @Security BearerAuth
// @Produce json
// @Success 200 {object} TURNCredentials
// @Router /calls/turn [get]
func (h *TURNHandler) GetTURNCredentials(c *gin.Context) {
	userID := c.GetString("user_id")

	creds, err := h.generateCredentials(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate credentials"})
		return
	}

	c.JSON(http.StatusOK, creds)
}

// generateCredentials — генерируем временные TURN credentials
//
// Схема (RFC 8489 / coturn use-auth-secret):
// username = "<unix_timestamp>:<userID>"
// password = base64(HMAC-SHA1(secret, username))
//
// coturn проверяет:
// 1. timestamp не истёк (TTL)
// 2. HMAC совпадает с secret
// Сервер никогда не хранит пароли — они вычисляются на лету
func (h *TURNHandler) generateCredentials(userID string) (*TURNCredentials, error) {
	// Timestamp истечения = сейчас + TTL
	expiresAt := time.Now().Unix() + int64(h.cfg.TTL)
	username := fmt.Sprintf("%d:%s", expiresAt, userID)

	// HMAC-SHA1(secret, username) → base64
	mac := hmac.New(sha1.New, []byte(h.cfg.AuthSecret))
	mac.Write([]byte(username))
	password := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	// Список серверов — сначала STUN (бесплатно), потом TURN (relay)
	uris := []string{
		// STUN — для P2P без NAT (бесплатно, нет relay)
		fmt.Sprintf("stun:%s:%d", h.cfg.Host, h.cfg.Port),
		// TURN UDP — основной
		fmt.Sprintf("turn:%s:%d?transport=udp", h.cfg.Host, h.cfg.Port),
		// TURN TCP — fallback если UDP заблокирован
		fmt.Sprintf("turn:%s:%d?transport=tcp", h.cfg.Host, h.cfg.Port),
		// TURNS (TLS) — для strict firewall (только 443/5349)
		fmt.Sprintf("turns:%s:%d?transport=tcp", h.cfg.Host, h.cfg.TLSPort),
	}

	return &TURNCredentials{
		Username: username,
		Password: password,
		TTL:      h.cfg.TTL,
		URIs:     uris,
	}, nil
}
package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ─────────────────────────────────────────────
// PoW — Proof of Work анти-бот защита
//
// Flow:
//   1. GET  /auth/pow/challenge → {challenge, difficulty, expires_at}
//   2. Клиент ищет nonce: SHA256(challenge+nonce) начинается с difficulty нулей
//   3. POST /auth/register + pow_challenge + pow_nonce
//
// Difficulty 4 = ~10k попыток = ~50ms на современном железе
// Для ботов при 1000 регистраций/сек = 50 CPU-секунд
// ─────────────────────────────────────────────

const (
	powDifficulty = 4               // кол-во ведущих нулей в hex SHA256
	powTTL        = 10 * time.Minute // время жизни challenge
	powPrefix     = "pow:"           // Redis key prefix
)

type PowChallenge struct {
	Challenge  string    `json:"challenge"`
	Difficulty int       `json:"difficulty"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// GetPowChallenge — выдаёт новый challenge
// GET /api/v1/auth/pow/challenge
func (h *AuthHandler) GetPowChallenge(c *gin.Context) {
	// Генерируем 32 случайных байта
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate challenge"})
		return
	}

	challenge := hex.EncodeToString(b)
	expiresAt := time.Now().Add(powTTL)

	// Сохраняем в Redis чтобы не принять один challenge дважды
	key := powPrefix + challenge
	if err := h.rdb.Set(c, key, "1", powTTL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store challenge"})
		return
	}

	c.JSON(http.StatusOK, PowChallenge{
		Challenge:  challenge,
		Difficulty: powDifficulty,
		ExpiresAt:  expiresAt,
	})
}

// VerifyPoW — проверяет решение PoW задачи
// Возвращает ошибку если решение неверное или challenge уже использован
func (h *AuthHandler) VerifyPoW(c *gin.Context, challenge, nonce string) error {
	if challenge == "" || nonce == "" {
		return fmt.Errorf("pow_challenge and pow_nonce are required")
	}

	// Проверяем что challenge существует в Redis (не истёк и не использован)
	key := powPrefix + challenge
	exists, err := h.rdb.Exists(c, key)
	if err != nil || !exists {
		return fmt.Errorf("invalid or expired challenge")
	}

	// Проверяем решение: SHA256(challenge+nonce) должен начинаться с N нулей
	hash := sha256.Sum256([]byte(challenge + nonce))
	hashHex := hex.EncodeToString(hash[:])

	prefix := strings.Repeat("0", powDifficulty)
	if !strings.HasPrefix(hashHex, prefix) {
		return fmt.Errorf("invalid proof of work solution")
	}

	// Удаляем challenge — one-time use
	h.rdb.Delete(c, key)

	return nil
}
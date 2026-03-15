package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/corvus-ch/shamir"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	db "messenger/internal/db/sqlc"
)

// SocialRecoveryHandler — восстановление аккаунта через доверенных контактов
// Использует Shamir's Secret Sharing:
//   - Пользователь задаёт N guardians и порог K
//   - Секрет делится на N частей, каждый guardian получает одну
//   - Для восстановления нужно K из N частей
type SocialRecoveryHandler struct {
	q     *db.Queries
	sqlDB *sql.DB
}

func NewSocialRecoveryHandler(sqlDB *sql.DB) *SocialRecoveryHandler {
	return &SocialRecoveryHandler{
		q:     db.New(sqlDB),
		sqlDB: sqlDB,
	}
}

// ─────────────────────────────────────────────
// POST /api/v1/recovery/setup
// Создаёт сессию восстановления и делит секрет
// ─────────────────────────────────────────────

type setupRecoveryReq struct {
	GuardianIDs []string `json:"guardian_ids" binding:"required,min=2,max=10"`
	Threshold   int      `json:"threshold" binding:"required,min=2"`
}

type setupRecoveryResp struct {
	SessionID string            `json:"session_id"`
	Threshold int               `json:"threshold"`
	Total     int               `json:"total"`
	Shares    map[string]string `json:"shares"` // guardianID → зашифрованный шард
}

// SetupRecovery — создаёт сессию и раздаёт шарды guardians
// POST /api/v1/recovery/setup
func (h *SocialRecoveryHandler) SetupRecovery(c *gin.Context) {
	userID := c.MustGet("user_id").(uuid.UUID)

	var req setupRecoveryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Threshold > len(req.GuardianIDs) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("threshold (%d) cannot exceed number of guardians (%d)",
				req.Threshold, len(req.GuardianIDs)),
		})
		return
	}

	// Генерируем 32-байтовый секрет
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate secret"})
		return
	}

	// Делим секрет на шарды через Shamir's Secret Sharing
shares, err := shamir.Split(secret, len(req.GuardianIDs), req.Threshold)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to split secret"})
		return
	}

	// Хешируем секрет для верификации (не храним сам секрет)
	hash := sha256.Sum256(secret)
	encryptedSecret := hex.EncodeToString(hash[:])

	// Создаём сессию восстановления
	sessionID := uuid.New()
	_, err = h.sqlDB.ExecContext(c, `
		INSERT INTO recovery_sessions (id, user_id, threshold, total_shares, encrypted_secret)
		VALUES ($1, $2, $3, $4, $5)
	`, sessionID, userID, req.Threshold, len(req.GuardianIDs), encryptedSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create recovery session"})
		return
	}

	// Раздаём шарды guardians
	shareMap := make(map[string]string)
	for i, guardianIDStr := range req.GuardianIDs {
		guardianID, err := uuid.Parse(guardianIDStr)
		if err != nil {
			continue
		}

		// Кодируем шард в base64
		shareBytes := shares[byte(i+1)]
		shareB64 := base64.StdEncoding.EncodeToString(shareBytes)

		// Сохраняем шард
		_, err = h.sqlDB.ExecContext(c, `
			INSERT INTO recovery_shares (session_id, guardian_id, share_index, share_data)
			VALUES ($1, $2, $3, $4)
		`, sessionID, guardianID, i+1, shareB64)
		if err != nil {
			continue
		}

		shareMap[guardianIDStr] = shareB64
	}

	// Обновляем статус сессии
	h.sqlDB.ExecContext(c, `
		UPDATE recovery_sessions SET status = 'active' WHERE id = $1
	`, sessionID)

	c.JSON(http.StatusCreated, setupRecoveryResp{
		SessionID: sessionID.String(),
		Threshold: req.Threshold,
		Total:     len(req.GuardianIDs),
		Shares:    shareMap,
	})
}

// ─────────────────────────────────────────────
// GET /api/v1/recovery/sessions
// Список активных сессий восстановления юзера
// ─────────────────────────────────────────────

func (h *SocialRecoveryHandler) GetSessions(c *gin.Context) {
	userID := c.MustGet("user_id").(uuid.UUID)

	rows, err := h.sqlDB.QueryContext(c, `
		SELECT id, threshold, total_shares, status, created_at, expires_at
		FROM recovery_sessions
		WHERE user_id = $1 AND status = 'active'
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type sessionRow struct {
		ID          string    `json:"id"`
		Threshold   int       `json:"threshold"`
		TotalShares int       `json:"total_shares"`
		Status      string    `json:"status"`
		CreatedAt   time.Time `json:"created_at"`
		ExpiresAt   time.Time `json:"expires_at"`
	}

	var sessions []sessionRow
	for rows.Next() {
		var s sessionRow
		if err := rows.Scan(&s.ID, &s.Threshold, &s.TotalShares, &s.Status, &s.CreatedAt, &s.ExpiresAt); err != nil {
			continue
		}
		sessions = append(sessions, s)
	}

	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

// ─────────────────────────────────────────────
// GET /api/v1/recovery/shares
// Guardian видит свои шарды (для передачи при восстановлении)
// ─────────────────────────────────────────────

func (h *SocialRecoveryHandler) GetMyShares(c *gin.Context) {
	guardianID := c.MustGet("user_id").(uuid.UUID)

	rows, err := h.sqlDB.QueryContext(c, `
		SELECT rs.id, rs.session_id, rs.share_index, rs.share_data,
		       u.username, rs.created_at
		FROM recovery_shares rs
		JOIN recovery_sessions sess ON sess.id = rs.session_id
		JOIN users u ON u.id = sess.user_id
		WHERE rs.guardian_id = $1
		  AND sess.status = 'active'
		  AND sess.expires_at > NOW()
	`, guardianID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type shareRow struct {
		ID        string    `json:"id"`
		SessionID string    `json:"session_id"`
		Index     int       `json:"share_index"`
		ShareData string    `json:"share_data"`
		Username  string    `json:"owner_username"`
		CreatedAt time.Time `json:"created_at"`
	}

	var shares []shareRow
	for rows.Next() {
		var s shareRow
		if err := rows.Scan(&s.ID, &s.SessionID, &s.Index, &s.ShareData, &s.Username, &s.CreatedAt); err != nil {
			continue
		}
		shares = append(shares, s)
	}

	c.JSON(http.StatusOK, gin.H{"shares": shares})
}

// ─────────────────────────────────────────────
// POST /api/v1/recovery/recover
// Восстанавливает секрет из K шардов
// ─────────────────────────────────────────────

type recoverReq struct {
	SessionID string            `json:"session_id" binding:"required"`
	Shares    map[string]string `json:"shares" binding:"required"` // index → shareData
}

func (h *SocialRecoveryHandler) Recover(c *gin.Context) {
	var req recoverReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
		return
	}

	// Получаем сессию
	var threshold int
	var encryptedSecret string
	var status string
	err = h.sqlDB.QueryRowContext(c, `
		SELECT threshold, encrypted_secret, status
		FROM recovery_sessions
		WHERE id = $1 AND expires_at > NOW()
	`, sessionID).Scan(&threshold, &encryptedSecret, &status)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found or expired"})
		return
	}
	if status != "active" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session is not active"})
		return
	}

	if len(req.Shares) < threshold {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("need at least %d shares, got %d", threshold, len(req.Shares)),
		})
		return
	}

	// Декодируем шарды
	shamirShares := make(map[byte][]byte)
	for indexStr, shareB64 := range req.Shares {
		var index int
		if _, err := fmt.Sscanf(indexStr, "%d", &index); err != nil {
			continue
		}
		shareBytes, err := base64.StdEncoding.DecodeString(shareB64)
		if err != nil {
			continue
		}
		shamirShares[byte(index)] = shareBytes
	}

	// Восстанавливаем секрет
	secret, err := shamir.Combine(shamirShares)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to recover secret: invalid shares"})
		return
	}

	// Верифицируем секрет
	hash := sha256.Sum256(secret)
	hashHex := hex.EncodeToString(hash[:])

	if hashHex != encryptedSecret {
		c.JSON(http.StatusBadRequest, gin.H{"error": "secret verification failed"})
		return
	}

	// Помечаем сессию как восстановленную
	h.sqlDB.ExecContext(c, `
		UPDATE recovery_sessions SET status = 'recovered' WHERE id = $1
	`, sessionID)

	// Возвращаем секрет клиенту (он использует его для расшифровки локального бэкапа)
	c.JSON(http.StatusOK, gin.H{
		"secret":     base64.StdEncoding.EncodeToString(secret),
		"session_id": req.SessionID,
		"message":    "secret recovered successfully",
	})
}

// ─────────────────────────────────────────────
// DELETE /api/v1/recovery/sessions/:id
// Отменяет сессию восстановления
// ─────────────────────────────────────────────

func (h *SocialRecoveryHandler) CancelSession(c *gin.Context) {
	userID := c.MustGet("user_id").(uuid.UUID)
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
		return
	}

	res, err := h.sqlDB.ExecContext(c, `
		UPDATE recovery_sessions SET status = 'cancelled'
		WHERE id = $1 AND user_id = $2
	`, sessionID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "session cancelled"})
}

// ─── helpers ─────────────────────────────────

func jsonMarshalShares(shares map[string]string) []byte {
	b, _ := json.Marshal(shares)
	return b
}

// Заглушка для компилятора
var _ = jsonMarshalShares

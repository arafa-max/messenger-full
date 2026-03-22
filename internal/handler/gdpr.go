package handler

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type GDPRHandler struct {
	db *sql.DB
}

func NewGDPRHandler(db *sql.DB) *GDPRHandler {
	return &GDPRHandler{db: db}
}

// ExportData godoc
// @Summary Экспорт всех данных пользователя (GDPR Art. 20)
// @Tags GDPR
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]any
// @Router /gdpr/export [get]
func (h *GDPRHandler) ExportData(c *gin.Context) {
	userID := c.GetInt64("userID")

	// ── Профиль ───────────────────────────────────────────────
	var profile struct {
		ID        int64     `json:"id"`
		Username  string    `json:"username"`
		CreatedAt time.Time `json:"created_at"`
	}
	err := h.db.QueryRowContext(c.Request.Context(),
		`SELECT id, username, created_at FROM users WHERE id = $1`, userID,
	).Scan(&profile.ID, &profile.Username, &profile.CreatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch profile"})
		return
	}

	// ── Сообщения ─────────────────────────────────────────────
	rows, err := h.db.QueryContext(c.Request.Context(),
		`SELECT id, chat_id, content, created_at
		 FROM messages
		 WHERE sender_id = $1
		 ORDER BY created_at DESC
		 LIMIT 1000`, userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch messages"})
		return
	}
	defer rows.Close()

	type msgRow struct {
		ID        int64     `json:"id"`
		ChatID    int64     `json:"chat_id"`
		Content   string    `json:"content"`
		CreatedAt time.Time `json:"created_at"`
	}
	var messages []msgRow
	for rows.Next() {
		var m msgRow
		if err := rows.Scan(&m.ID, &m.ChatID, &m.Content, &m.CreatedAt); err != nil {
			continue
		}
		messages = append(messages, m)
	}

	// ── Сессии ────────────────────────────────────────────────
	sRows, err := h.db.QueryContext(c.Request.Context(),
		`SELECT id, created_at, expires_at
		 FROM sessions
		 WHERE user_id = $1
		 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch sessions"})
		return
	}
	defer sRows.Close()

	type sessionRow struct {
		ID        int64     `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	var sessions []sessionRow
	for sRows.Next() {
		var s sessionRow
		if err := sRows.Scan(&s.ID, &s.CreatedAt, &s.ExpiresAt); err != nil {
			continue
		}
		sessions = append(sessions, s)
	}

	c.JSON(http.StatusOK, gin.H{
		"exported_at": time.Now().UTC(),
		"profile":     profile,
		"messages":    messages,
		"sessions":    sessions,
	})
}

// DeleteAccount godoc
// @Summary Удалить аккаунт и все данные (GDPR Art. 17)
// @Tags GDPR
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]string
// @Router /gdpr/delete [delete]
func (h *GDPRHandler) DeleteAccount(c *gin.Context) {
	userID := c.GetInt64("userID")

	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start transaction"})
		return
	}
	defer tx.Rollback()

	// Удаляем в правильном порядке (foreign keys)
	queries := []string{
		`DELETE FROM sessions          WHERE user_id = $1`,
		`DELETE FROM refresh_tokens    WHERE user_id = $1`,
		`DELETE FROM reactions         WHERE user_id = $1`,
		`DELETE FROM message_reads     WHERE user_id = $1`,
		`DELETE FROM chat_members      WHERE user_id = $1`,
		`DELETE FROM stories           WHERE user_id = $1`,
		`DELETE FROM story_views       WHERE user_id = $1`,
		`DELETE FROM user_prekeys      WHERE user_id = $1`,
		`DELETE FROM user_signed_prekeys WHERE user_id = $1`,
		`DELETE FROM user_identity_keys  WHERE user_id = $1`,
		// Сообщения — анонимизируем вместо удаления (сохраняем историю чата)
		`UPDATE messages SET content = '[deleted]', sender_id = NULL WHERE sender_id = $1`,
		// Наконец удаляем пользователя
		`DELETE FROM users WHERE id = $1`,
	}

	for _, q := range queries {
		if _, err := tx.ExecContext(c.Request.Context(), q, userID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete data"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to commit"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Your account and all associated data have been permanently deleted.",
	})
}
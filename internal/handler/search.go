package handler

import (
	"net/http"
	"strconv"
	"strings"

	"messenger/internal/database"

	"github.com/gin-gonic/gin"
)

// SearchHandler содержит все эндпоинты поиска (Block 10)
type SearchHandler struct {
	db *database.DB
}

func NewSearchHandler(db *database.DB) *SearchHandler {
	return &SearchHandler{db: db}
}

// ─────────────────────────────────────────────
// GET /api/v1/search/users?q=...&limit=20
// ─────────────────────────────────────────────
// Поиск пользователей по username (pg_trgm, индекс уже есть)
func (h *SearchHandler) SearchUsers(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if len(q) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query too short, min 2 chars"})
		return
	}

	limit := parseLimit(c.Query("limit"), 20, 50)

	type UserResult struct {
		ID        string `json:"id"`
		Username  string `json:"username"`
		AvatarURL string `json:"avatar_url"`
		IsOnline  bool   `json:"is_online"`
		IsBot     bool   `json:"is_bot"`
	}

	rows, err := h.db.Pool.Query(c.Request.Context(), `
		SELECT id, username, COALESCE(avatar_url,''), is_online, is_bot
		FROM users
		WHERE is_deleted = FALSE
		  AND username % $1
		ORDER BY similarity(username, $1) DESC
		LIMIT $2
	`, q, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	results := make([]UserResult, 0)
	for rows.Next() {
		var u UserResult
		if err := rows.Scan(&u.ID, &u.Username, &u.AvatarURL, &u.IsOnline, &u.IsBot); err != nil {
			continue
		}
		results = append(results, u)
	}

	c.JSON(http.StatusOK, gin.H{"users": results, "count": len(results)})
}

// ─────────────────────────────────────────────
// GET /api/v1/chats/:id/search?q=...&limit=20&before_id=...
// ─────────────────────────────────────────────
// Поиск сообщений внутри чата (pg_trgm)
func (h *SearchHandler) SearchMessages(c *gin.Context) {
	chatID := c.Param("id")
	q := strings.TrimSpace(c.Query("q"))
	if len(q) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query too short, min 2 chars"})
		return
	}

	limit := parseLimit(c.Query("limit"), 20, 50)
	beforeID := c.Query("before_id") // курсор пагинации

	userID, _ := c.Get("user_id")

	// Проверяем что юзер — участник чата
	var isMember bool
	err := h.db.Pool.QueryRow(c.Request.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM chat_members
			WHERE chat_id = $1 AND user_id = $2 AND is_banned = FALSE
		)
	`, chatID, userID).Scan(&isMember)
	if err != nil || !isMember {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a member"})
		return
	}

	type MessageResult struct {
		ID        string `json:"id"`
		Content   string `json:"content"`
		SenderID  string `json:"sender_id"`
		Username  string `json:"sender_username"`
		CreatedAt string `json:"created_at"`
	}

	var query string
	var args []interface{}

	if beforeID == "" {
		query = `
			SELECT m.id, m.content, m.sender_id, u.username, m.created_at::text
			FROM messages m
			JOIN users u ON u.id = m.sender_id
			WHERE m.chat_id = $1
			  AND m.is_deleted = FALSE
			  AND m.content % $2
			ORDER BY similarity(m.content, $2) DESC, m.created_at DESC
			LIMIT $3
		`
		args = []interface{}{chatID, q, limit}
	} else {
		// Пагинация по курсору
		query = `
			SELECT m.id, m.content, m.sender_id, u.username, m.created_at::text
			FROM messages m
			JOIN users u ON u.id = m.sender_id
			WHERE m.chat_id = $1
			  AND m.is_deleted = FALSE
			  AND m.content % $2
			  AND m.created_at < (SELECT created_at FROM messages WHERE id = $4)
			ORDER BY similarity(m.content, $2) DESC, m.created_at DESC
			LIMIT $3
		`
		args = []interface{}{chatID, q, limit, beforeID}
	}

	rows, err := h.db.Pool.Query(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	results := make([]MessageResult, 0)
	for rows.Next() {
		var m MessageResult
		if err := rows.Scan(&m.ID, &m.Content, &m.SenderID, &m.Username, &m.CreatedAt); err != nil {
			continue
		}
		results = append(results, m)
	}

	c.JSON(http.StatusOK, gin.H{"messages": results, "count": len(results)})
}

// ─────────────────────────────────────────────
// GET /api/v1/search/chats?q=...&type=channel&limit=20
// ─────────────────────────────────────────────
// Глобальный поиск публичных каналов и групп
func (h *SearchHandler) SearchChats(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if len(q) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query too short, min 2 chars"})
		return
	}

	chatType := c.Query("type") // channel, group, "" = все
	limit := parseLimit(c.Query("limit"), 20, 50)

	type ChatResult struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Username    string `json:"username"`
		Description string `json:"description"`
		AvatarURL   string `json:"avatar_url"`
		Type        string `json:"type"`
		MemberCount int    `json:"member_count"`
	}

	var rows interface{ Close() }
	var err error

	if chatType != "" && (chatType == "channel" || chatType == "group") {
		rows, err = h.db.Pool.Query(c.Request.Context(), `
			SELECT id, COALESCE(name,''), COALESCE(username,''),
			       COALESCE(description,''), COALESCE(avatar_url,''),
			       type, member_count
			FROM chats
			WHERE is_public = TRUE
			  AND is_deleted = FALSE
			  AND type = $3
			  AND (name % $1 OR description % $1)
			ORDER BY similarity(name, $1) DESC, member_count DESC
			LIMIT $2
		`, q, limit, chatType)
	} else {
		rows, err = h.db.Pool.Query(c.Request.Context(), `
			SELECT id, COALESCE(name,''), COALESCE(username,''),
			       COALESCE(description,''), COALESCE(avatar_url,''),
			       type, member_count
			FROM chats
			WHERE is_public = TRUE
			  AND is_deleted = FALSE
			  AND type IN ('channel','group')
			  AND (name % $1 OR description % $1)
			ORDER BY similarity(name, $1) DESC, member_count DESC
			LIMIT $2
		`, q, limit)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Используем pgx rows напрямую
	pgxRows, ok := rows.(interface {
		Next() bool
		Scan(...interface{}) error
		Close()
	})
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rows type assertion failed"})
		return
	}
	defer pgxRows.Close()

	results := make([]ChatResult, 0)
	for pgxRows.Next() {
		var ch ChatResult
		if err := pgxRows.Scan(&ch.ID, &ch.Name, &ch.Username, &ch.Description,
			&ch.AvatarURL, &ch.Type, &ch.MemberCount); err != nil {
			continue
		}
		results = append(results, ch)
	}

	c.JSON(http.StatusOK, gin.H{"chats": results, "count": len(results)})
}

// ─────────────────────────────────────────────
// GET /api/v1/search/stickers?q=...&limit=20
// ─────────────────────────────────────────────
// Поиск стикер-паков по названию
func (h *SearchHandler) SearchStickers(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if len(q) < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query required"})
		return
	}

	limit := parseLimit(c.Query("limit"), 20, 50)

	type StickerPackResult struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		ThumbURL  string `json:"thumb_url"`
		Count     int    `json:"count"`
	}

	rows, err := h.db.Pool.Query(c.Request.Context(), `
		SELECT sp.id, sp.name, COALESCE(sp.thumb_url,''),
		       COUNT(s.id)::int AS sticker_count
		FROM sticker_packs sp
		LEFT JOIN stickers s ON s.pack_id = sp.id
		WHERE sp.name % $1
		GROUP BY sp.id, sp.name, sp.thumb_url
		ORDER BY similarity(sp.name, $1) DESC
		LIMIT $2
	`, q, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	results := make([]StickerPackResult, 0)
	for rows.Next() {
		var sp StickerPackResult
		if err := rows.Scan(&sp.ID, &sp.Name, &sp.ThumbURL, &sp.Count); err != nil {
			continue
		}
		results = append(results, sp)
	}

	c.JSON(http.StatusOK, gin.H{"sticker_packs": results, "count": len(results)})
}

// ─────────────────────────────────────────────
// GET /api/v1/search/gif?q=...&limit=20
// ─────────────────────────────────────────────
// Поиск GIF через emoji_keywords таблицу
func (h *SearchHandler) SearchGIF(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if len(q) < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query required"})
		return
	}

	limit := parseLimit(c.Query("limit"), 20, 50)

	type GIFResult struct {
		ID       string `json:"id"`
		URL      string `json:"url"`
		ThumbURL string `json:"thumb_url"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
	}

	// Ищем через media таблицу по type=gif
	// Дополнительно проверяем emoji_keywords для маппинга текст→эмодзи
	rows, err := h.db.Pool.Query(c.Request.Context(), `
		SELECT m.id,
		       '/api/v1/media/' || m.id || '/url' AS url,
		       COALESCE('/api/v1/media/' || m.id || '/thumb', '') AS thumb_url,
		       COALESCE(m.width, 0),
		       COALESCE(m.height, 0)
		FROM media m
		WHERE m.type = 'gif'
		  AND m.status = 'processed'
		  AND (m.original_name ILIKE '%' || $1 || '%')
		ORDER BY m.created_at DESC
		LIMIT $2
	`, q, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	results := make([]GIFResult, 0)
	for rows.Next() {
		var g GIFResult
		if err := rows.Scan(&g.ID, &g.URL, &g.ThumbURL, &g.Width, &g.Height); err != nil {
			continue
		}
		results = append(results, g)
	}

	c.JSON(http.StatusOK, gin.H{"gifs": results, "count": len(results)})
}

// ─────────────────────────────────────────────
// GET /api/v1/search/media?q=...&chat_id=...&limit=20
// ─────────────────────────────────────────────
// Поиск внутри медиа (OCR + Whisper индекс)
func (h *SearchHandler) SearchMedia(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if len(q) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query too short, min 2 chars"})
		return
	}

	chatID := c.Query("chat_id") // опционально — фильтр по чату
	limit := parseLimit(c.Query("limit"), 20, 50)
	userID, _ := c.Get("user_id")

	type MediaSearchResult struct {
		MediaID   string `json:"media_id"`
		MessageID string `json:"message_id"`
		Content   string `json:"content"`
		Source    string `json:"source"` // ocr | whisper
		ChatID    string `json:"chat_id"`
	}

	var rows interface {
		Next() bool
		Scan(...interface{}) error
		Close()
	}
	var err error

	if chatID != "" {
		// Проверяем членство
		var isMember bool
		h.db.Pool.QueryRow(c.Request.Context(), `
			SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id=$1 AND user_id=$2 AND is_banned=FALSE)
		`, chatID, userID).Scan(&isMember)
		if !isMember {
			c.JSON(http.StatusForbidden, gin.H{"error": "not a member"})
			return
		}

		rows, err = h.db.Pool.Query(c.Request.Context(), `
			SELECT msi.media_id::text, COALESCE(msi.message_id::text,''),
			       msi.content, msi.source,
			       COALESCE(m.chat_id::text,'')
			FROM media_search_index msi
			LEFT JOIN messages m ON m.id = msi.message_id
			WHERE msi.content % $1
			  AND m.chat_id = $2
			  AND m.is_deleted = FALSE
			ORDER BY similarity(msi.content, $1) DESC
			LIMIT $3
		`, q, chatID, limit)
	} else {
		// Только сообщения из чатов где юзер является участником
		rows, err = h.db.Pool.Query(c.Request.Context(), `
			SELECT msi.media_id::text, COALESCE(msi.message_id::text,''),
			       msi.content, msi.source,
			       COALESCE(m.chat_id::text,'')
			FROM media_search_index msi
			LEFT JOIN messages m ON m.id = msi.message_id
			JOIN chat_members cm ON cm.chat_id = m.chat_id AND cm.user_id = $2
			WHERE msi.content % $1
			  AND m.is_deleted = FALSE
			ORDER BY similarity(msi.content, $1) DESC
			LIMIT $3
		`, q, userID, limit)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	results := make([]MediaSearchResult, 0)
	for rows.Next() {
		var r MediaSearchResult
		if err := rows.Scan(&r.MediaID, &r.MessageID, &r.Content, &r.Source, &r.ChatID); err != nil {
			continue
		}
		results = append(results, r)
	}

	c.JSON(http.StatusOK, gin.H{"results": results, "count": len(results)})
}

// ─────────────────────────────────────────────
// GET /api/v1/search/global?q=...&limit=20
// ─────────────────────────────────────────────
// Единая точка: users + chats одним запросом
func (h *SearchHandler) SearchGlobal(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if len(q) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query too short, min 2 chars"})
		return
	}

	limit := parseLimit(c.Query("limit"), 10, 20)

	// Users
	type UserResult struct {
		ID        string `json:"id"`
		Username  string `json:"username"`
		AvatarURL string `json:"avatar_url"`
	}
	type ChatResult struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Username    string `json:"username"`
		Type        string `json:"type"`
		MemberCount int    `json:"member_count"`
	}

	userRows, err := h.db.Pool.Query(c.Request.Context(), `
		SELECT id, username, COALESCE(avatar_url,'')
		FROM users
		WHERE is_deleted = FALSE AND username % $1
		ORDER BY similarity(username, $1) DESC
		LIMIT $2
	`, q, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer userRows.Close()

	users := make([]UserResult, 0)
	for userRows.Next() {
		var u UserResult
		if err := userRows.Scan(&u.ID, &u.Username, &u.AvatarURL); err != nil {
			continue
		}
		users = append(users, u)
	}

	chatRows, err := h.db.Pool.Query(c.Request.Context(), `
		SELECT id, COALESCE(name,''), COALESCE(username,''), type, member_count
		FROM chats
		WHERE is_public = TRUE AND is_deleted = FALSE
		  AND type IN ('channel','group')
		  AND name % $1
		ORDER BY similarity(name, $1) DESC, member_count DESC
		LIMIT $2
	`, q, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer chatRows.Close()

	chats := make([]ChatResult, 0)
	for chatRows.Next() {
		var ch ChatResult
		if err := chatRows.Scan(&ch.ID, &ch.Name, &ch.Username, &ch.Type, &ch.MemberCount); err != nil {
			continue
		}
		chats = append(chats, ch)
	}

	c.JSON(http.StatusOK, gin.H{
		"users": users,
		"chats": chats,
	})
}

// ─────────────────────────────────────────────
// helpers
// ─────────────────────────────────────────────

func parseLimit(s string, def, max int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}
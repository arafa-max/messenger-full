package handler

import (
	"database/sql"
	"net/http"
	"unicode"

	db "messenger/internal/db/sqlc"
	"messenger/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type StickerHandler struct {
	queries *db.Queries
	minio   *storage.MinIOClient
}

func NewStickerHandler(q *db.Queries, m *storage.MinIOClient) *StickerHandler {
	return &StickerHandler{queries: q, minio: m}
}

func (h *StickerHandler) GetMyPacks(c *gin.Context) {
	userID := c.MustGet("user_id").(uuid.UUID)
	rows, err := h.queries.GetUserStickerPacks(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (h *StickerHandler) GetPublicPacks(c *gin.Context) {
	userID := c.MustGet("user_id").(uuid.UUID)

	isPremium := false
	if sub, err := h.queries.GetSubscriptionByUserID(c, userID); err == nil && sub.Status == "active" {
		isPremium = true
	}

	if isPremium {
		packs, err := h.queries.GetPremiumStickerPacks(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get packs"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"packs": packs})
		return
	}

	packs, err := h.queries.GetPublicStickerPacks(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get packs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"packs": packs})
}
func (h *StickerHandler) InstallPack(c *gin.Context) {
	userID := c.MustGet("user_id").(uuid.UUID)
	packID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pack id"})
		return
	}
	err = h.queries.InstallStickerPack(c.Request.Context(), db.InstallStickerPackParams{
		UserID: userID,
		PackID: packID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *StickerHandler) UninstallPack(c *gin.Context) {
	userID := c.MustGet("user_id").(uuid.UUID)
	packID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pack id"})
		return
	}
	err = h.queries.UninstallStickerPack(c.Request.Context(), db.UninstallStickerPackParams{
		UserID: userID,
		PackID: packID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *StickerHandler) GetPackStickers(c *gin.Context) {
	packID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pack id"})
		return
	}
	rows, err := h.queries.GetPackStickers(c.Request.Context(), packID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rows)
}

// SearchStickers godoc
// @Summary      Поиск стикеров по тексту/emoji
// @Tags         stickers
// @Security     BearerAuth
// @Param        q query string true "Поисковый запрос"
// @Success      200 {array} db.SearchStickersRow
// @Router       /stickers/search [get]
func (h *StickerHandler) SearchStickers(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query required"})
		return
	}
	rows, err := h.queries.SearchStickers(c.Request.Context(), sql.NullString{
		String: q,
		Valid:  true,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rows)
}

// SuggestStickers godoc
// @Summary      Inline suggestions: стикеры по эмодзи или тексту
// @Description  Два режима:
//  1. Вводишь эмодзи 😂 → сразу ищем стикеры по эмодзи
//  2. Вводишь текст "смеюсь" → находим эмодзи (😂) → ищем стикеры
//     Возвращает пустой массив если ничего не найдено.
//
// @Tags         stickers
// @Security     BearerAuth
// @Param        q query string true "Текст или эмодзи"
// @Success      200 {object} SuggestResponse
// @Router       /stickers/suggest [get]
func (h *StickerHandler) SuggestStickers(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusOK, suggestResponse(nil, ""))
		return
	}

	userID := c.MustGet("user_id").(uuid.UUID)
	ctx := c.Request.Context()

	// Режим 1: введён эмодзи → сразу ищем стикеры
	if containsEmoji(q) {
		rows, err := h.queries.GetStickersByEmoji(ctx, db.GetStickersByEmojiParams{
			Emoji:  sql.NullString{String: q, Valid: true},
			UserID: userID,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, suggestResponse(rows, q))
		return
	}

	// Режим 2: введён текст → ищем подходящие эмодзи через keyword mapping
	emojis, err := h.queries.GetEmojiByKeyword(ctx, sql.NullString{String: q, Valid: true})
	if err != nil || len(emojis) == 0 {
		c.JSON(http.StatusOK, suggestResponse(nil, ""))
		return
	}

	// Берём топ-1 эмодзи (самый релевантный) и ищем стикеры
	topEmoji := emojis[0].Emoji
	rows, err := h.queries.GetStickersByEmoji(ctx, db.GetStickersByEmojiParams{
		Emoji:  sql.NullString{String: topEmoji, Valid: true},
		UserID: userID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, suggestResponse(rows, topEmoji))
}

// SuggestResponse — ответ с найденными стикерами и эмодзи-триггером
type SuggestResponse struct {
	Emoji    string                     `json:"emoji"`    // какой эмодзи сматчился
	Stickers []db.GetStickersByEmojiRow `json:"stickers"` // стикеры
}

func suggestResponse(stickers []db.GetStickersByEmojiRow, emoji string) SuggestResponse {
	if stickers == nil {
		stickers = []db.GetStickersByEmojiRow{}
	}
	return SuggestResponse{Emoji: emoji, Stickers: stickers}
}

// containsEmoji — проверяет что строка содержит эмодзи символ.
// Telegram-подход: показываем стикеры только если ввод — эмодзи.
func containsEmoji(s string) bool {
	for _, r := range s {
		if isEmoji(r) {
			return true
		}
	}
	return false
}

func isEmoji(r rune) bool {
	return unicode.Is(unicode.So, r) || // Symbol, other (большинство эмодзи)
		(r >= 0x1F600 && r <= 0x1F64F) || // Emoticons
		(r >= 0x1F300 && r <= 0x1F5FF) || // Misc symbols
		(r >= 0x1F680 && r <= 0x1F6FF) || // Transport
		(r >= 0x1F900 && r <= 0x1F9FF) || // Supplemental
		(r >= 0x2600 && r <= 0x26FF) || // Misc symbols
		(r >= 0x2700 && r <= 0x27BF) // Dingbats
}

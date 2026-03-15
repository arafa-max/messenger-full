package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	db "messenger/internal/db/sqlc"
	"messenger/internal/storage"
)

type AnimatedEmojiHandler struct {
	queries *db.Queries
	minio   *storage.MinIOClient
}

func NewAnimatedEmojiHandler(q *db.Queries, m *storage.MinIOClient) *AnimatedEmojiHandler {
	return &AnimatedEmojiHandler{queries: q, minio: m}
}

// GetAnimatedEmoji godoc
// @Summary      Получить URL анимированного эмодзи
// @Description  Возвращает presigned URL на Lottie JSON файл в MinIO.
//               Клиент кэширует и проигрывает через lottie-react-native.
// @Tags         emoji
// @Security     BearerAuth
// @Param        e query string true "Эмодзи символ (например: 📥)"
// @Success      200 {object} animatedEmojiResponse
// @Router       /emoji/animated [get]
func (h *AnimatedEmojiHandler) GetAnimatedEmoji(c *gin.Context) {
	emoji := c.Query("e")
	if emoji == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "emoji required"})
		return
	}

	row, err := h.queries.GetAnimatedEmoji(c.Request.Context(), emoji)
	if err != nil {
		// Эмодзи не найден — возвращаем null, клиент покажет статичный
		c.JSON(http.StatusOK, gin.H{"url": nil})
		return
	}

	// Генерируем presigned URL на 1 час
	url, err := h.minio.PresignedGetURL(c.Request.Context(), row.ObjectKey, time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate url"})
		return
	}

	c.JSON(http.StatusOK, animatedEmojiResponse{
		Emoji:     emoji,
		URL:       url,
		ObjectKey: row.ObjectKey,
	})
}

// GetAnimatedEmojiBatch godoc
// @Summary      Получить URL нескольких анимированных эмодзи за раз
// @Description  Принимает список эмодзи через запятую, возвращает map emoji→url.
//               Используется для предзагрузки при открытии чата.
// @Tags         emoji
// @Security     BearerAuth
// @Param        e query string true "Эмодзи через запятую: 😂,❤️,👍"
// @Success      200 {object} map[string]string
// @Router       /emoji/animated/batch [get]
func (h *AnimatedEmojiHandler) GetAnimatedEmojiBatch(c *gin.Context) {
	raw := c.Query("e")
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "emoji list required"})
		return
	}

	emojis := strings.Split(raw, ",")
	if len(emojis) > 50 {
		emojis = emojis[:50] // максимум 50 за раз
	}

	// Конвертируем в []string для sqlc
	emojiList := make([]string, 0, len(emojis))
	for _, e := range emojis {
		e = strings.TrimSpace(e)
		if e != "" {
			emojiList = append(emojiList, e)
		}
	}

	rows, err := h.queries.GetAnimatedEmojiBatch(c.Request.Context(), emojiList)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make(map[string]string, len(rows))
	for _, row := range rows {
		url, err := h.minio.PresignedGetURL(c.Request.Context(), row.ObjectKey, time.Hour)
		if err != nil {
			continue
		}
		result[row.Emoji] = url
	}

	c.JSON(http.StatusOK, result)
}

type animatedEmojiResponse struct {
	Emoji     string `json:"emoji"`
	URL       string `json:"url"`        // presigned MinIO URL
	ObjectKey string `json:"object_key"` // для клиентского кэша
}
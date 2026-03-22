package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"messenger/internal/auth"
	"messenger/internal/config"
	db "messenger/internal/db/sqlc"
	"messenger/internal/oauth"
	"messenger/internal/redis"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const oauthStatePrefix = "oauth:state:"
const oauthStateTTL = 5 * time.Minute

type OAuthHandler struct {
	q         *db.Queries
	jwt       *auth.JWTManager
	cfg       *config.Config
	rdb       *redis.Client
	providers map[string]oauth.Provider
}

func NewOAuthHandler(sqlDB *sql.DB, cfg *config.Config, rdb *redis.Client) *OAuthHandler {
	baseURL := cfg.Server.PublicURL

	providers := map[string]oauth.Provider{}

	if cfg.OAuth.Google.ClientID != "" {
		providers["google"] = oauth.NewGoogle(
			cfg.OAuth.Google.ClientID,
			cfg.OAuth.Google.ClientSecret,
			baseURL+"/api/v1/auth/oauth/google/callback",
		)
	}
	if cfg.OAuth.GitHub.ClientID != "" {
		providers["github"] = oauth.NewGitHub(
			cfg.OAuth.GitHub.ClientID,
			cfg.OAuth.GitHub.ClientSecret,
			baseURL+"/api/v1/auth/oauth/github/callback",
		)
	}

	return &OAuthHandler{
		q:         db.New(sqlDB),
		jwt:       auth.NewJWTManager(cfg.JWT.AccessSecret, cfg.JWT.RefreshSecret, cfg.JWT.AccessMinutes, cfg.JWT.RefreshDays),
		cfg:       cfg,
		rdb:       rdb,
		providers: providers,
	}
}

// GET /api/v1/auth/oauth/:provider
// Редиректит пользователя на страницу авторизации провайдера
func (h *OAuthHandler) Redirect(c *gin.Context) {
	providerName := c.Param("provider")
	p, ok := h.providers[providerName]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown provider: " + providerName})
		return
	}

	state, err := generateState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state"})
		return
	}

	// Если запрос от авторизованного пользователя — сохраняем его ID в state
	// чтобы при callback привязать аккаунт, а не создать новый
	if uid, exists := c.Get("user_id"); exists {
		stateVal := fmt.Sprintf("link:%s", uid.(uuid.UUID).String())
		if err := h.rdb.SetOAuthState(c, oauthStatePrefix+state, stateVal, oauthStateTTL); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save state"})
			return
		}
	} else {
		if err := h.rdb.SetOAuthState(c, oauthStatePrefix+state, "login", oauthStateTTL); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save state"})
			return
		}
	}

	c.Redirect(http.StatusTemporaryRedirect, p.GetAuthURL(state))
}

// GET /api/v1/auth/oauth/:provider/callback
// Провайдер редиректит сюда с code и state
func (h *OAuthHandler) Callback(c *gin.Context) {
	providerName := c.Param("provider")
	p, ok := h.providers[providerName]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown provider"})
		return
	}

	// Проверяем state (CSRF защита)
	state := c.Query("state")
	stateVal, err := h.rdb.GetOAuthState(c, oauthStatePrefix+state)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired state"})
		return
	}
	h.rdb.DelOAuthState(c, oauthStatePrefix+state)

	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing code"})
		return
	}

	// Меняем code на UserInfo
	userInfo, err := p.ExchangeCode(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "oauth exchange failed: " + err.Error()})
		return
	}

	// Режим привязки к существующему аккаунту
	if strings.HasPrefix(stateVal, "link:") {
		userIDStr := strings.TrimPrefix(stateVal, "link:")
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id in state"})
			return
		}
		if err := h.q.LinkOAuthAccount(c, db.LinkOAuthAccountParams{
			UserID:     userID,
			Provider:   providerName,
			ProviderID: userInfo.ProviderID,
			Email:      sql.NullString{String: userInfo.Email, Valid: userInfo.Email != ""},
			AvatarUrl:  sql.NullString{String: userInfo.Avatar, Valid: userInfo.Avatar != ""},
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to link account"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "account linked", "provider": providerName})
		return
	}

	// Режим логина/регистрации
	user, err := h.findOrCreateUser(c, userInfo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	tokens, err := h.issueOAuthTokens(c, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue tokens"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user":   toSafeUser(user),
		"tokens": tokens,
	})
}

// GET /api/v1/auth/oauth/accounts
// Список привязанных OAuth аккаунтов (требует JWT)
func (h *OAuthHandler) GetLinkedAccounts(c *gin.Context) {
	uid := c.MustGet("user_id").(uuid.UUID)

	accounts, err := h.q.GetOAuthAccounts(c, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get accounts"})
		return
	}

	type accountInfo struct {
		Provider string    `json:"provider"`
		Email    string    `json:"email,omitempty"`
		LinkedAt time.Time `json:"linked_at"`
	}
	result := make([]accountInfo, 0, len(accounts))
	for _, a := range accounts {
		ai := accountInfo{
			Provider: a.Provider,
			LinkedAt: a.CreatedAt,
		}
		if a.Email.Valid {
			ai.Email = a.Email.String
		}
		result = append(result, ai)
	}
	c.JSON(http.StatusOK, result)
}

// DELETE /api/v1/auth/oauth/:provider
// Отвязать OAuth аккаунт (требует JWT)
func (h *OAuthHandler) UnlinkAccount(c *gin.Context) {
	uid := c.MustGet("user_id").(uuid.UUID)
	providerName := c.Param("provider")

	// Проверяем что у пользователя есть пароль или другой OAuth — нельзя остаться без входа
	user, err := h.q.GetUserByID(c, uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if user.Password == "" {
		accounts, _ := h.q.GetOAuthAccounts(c, uid)
		if len(accounts) <= 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot unlink last login method"})
			return
		}
	}

	if err := h.q.UnlinkOAuthAccount(c, db.UnlinkOAuthAccountParams{
		UserID:   uid,
		Provider: providerName,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unlink"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "unlinked"})
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func (h *OAuthHandler) findOrCreateUser(c *gin.Context, info *oauth.UserInfo) (db.User, error) {
	// 1. Ищем по oauth_accounts
	existing, err := h.q.GetUserByOAuth(c, db.GetUserByOAuthParams{
		Provider:   info.Provider,
		ProviderID: info.ProviderID,
	})
	if err == nil {
		return existing, nil
	}

	// 2. Ищем по email (пользователь уже зарегистрирован обычно)
	if info.Email != "" {
		byEmail, err := h.q.GetUserByEmail(c, sql.NullString{String: info.Email, Valid: true})
		if err == nil {
			// Привязываем OAuth к существующему аккаунту
			_ = h.q.LinkOAuthAccount(c, db.LinkOAuthAccountParams{
				UserID:     byEmail.ID,
				Provider:   info.Provider,
				ProviderID: info.ProviderID,
				Email:      sql.NullString{String: info.Email, Valid: true},
				AvatarUrl:  sql.NullString{String: info.Avatar, Valid: info.Avatar != ""},
			})
			return byEmail, nil
		}
	}

	// 3. Создаём нового пользователя
	username := generateUsername(info)
	user, err := h.q.CreateUser(c, db.CreateUserParams{
		Username: username,
		Email:    sql.NullString{String: info.Email, Valid: info.Email != ""},
		Password: "", // нет пароля у OAuth пользователей
		Language: sql.NullString{String: "ru", Valid: true},
	})
	if err != nil {
		return db.User{}, fmt.Errorf("create user: %w", err)
	}

	// Обновляем аватар если есть
	if info.Avatar != "" {
		_ = h.q.UpdateUserAvatar(c, db.UpdateUserAvatarParams{
			ID:        user.ID,
			AvatarUrl: sql.NullString{String: info.Avatar, Valid: true},
		})
	}

	// Привязываем OAuth
	_ = h.q.LinkOAuthAccount(c, db.LinkOAuthAccountParams{
		UserID:     user.ID,
		Provider:   info.Provider,
		ProviderID: info.ProviderID,
		Email:      sql.NullString{String: info.Email, Valid: info.Email != ""},
		AvatarUrl:  sql.NullString{String: info.Avatar, Valid: info.Avatar != ""},
	})

	return user, nil
}

func (h *OAuthHandler) issueOAuthTokens(c *gin.Context, userID uuid.UUID) (*tokenPair, error) {
	access, err := h.jwt.GenerateAccess(userID)
	if err != nil {
		return nil, err
	}
	refresh, err := h.jwt.GenerateRefresh(userID)
	if err != nil {
		return nil, err
	}
	_, err = h.q.CreateSession(c, db.CreateSessionParams{
		UserID:       userID,
		RefreshToken: refresh,
		IpAddress:    parseInet(c.ClientIP()),
		ExpiresAt:    time.Now().AddDate(0, 0, h.cfg.JWT.RefreshDays),
	})
	if err != nil {
		return nil, err
	}
	return &tokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    h.cfg.JWT.AccessMinutes * 60,
	}, nil
}

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func generateUsername(info *oauth.UserInfo) string {
	base := strings.ToLower(strings.ReplaceAll(info.Name, " ", "_"))
	if base == "" {
		base = info.Provider + "_user"
	}
	// добавляем суффикс чтобы избежать коллизий
	suffix := make([]byte, 3)
	rand.Read(suffix)
	return fmt.Sprintf("%s_%s", base, hex.EncodeToString(suffix))
}

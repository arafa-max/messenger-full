package handler

import (
	"database/sql"
	"log"
	"messenger/internal/auth"
	"messenger/internal/config"
	db "messenger/internal/db/sqlc"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
)

type AuthHandler struct {
	q   *db.Queries
	jwt *auth.JWTManager
	cfg *config.Config
}

func NewAuthHandler(sqlDB *sql.DB, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		q:   db.New(sqlDB),
		jwt: auth.NewJWTManager(cfg.JWT.AccessSecret, cfg.JWT.RefreshSecret, cfg.JWT.AccessMinutes, cfg.JWT.RefreshDays),
		cfg: cfg,
	}
}

type registerReq struct {
	Username string `json:"username" binding:"required,min=3,max=32"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

// @Summary Registration
// @Tags auth
// @Accept json
// @Produce json
// @Param body body registerReq true "data registraions"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := h.q.GetUserByUsername(c, req.Username); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
		return
	}
	if _, err := h.q.GetUserByEmail(c, sql.NullString{String: req.Email, Valid: true}); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "email already taken"})
		return
	}
	hashed, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.q.CreateUser(c, db.CreateUserParams{
		Username: req.Username,
		Email:    sql.NullString{String: req.Email, Valid: true},
		Password: hashed,
		Language: sql.NullString{String: "ru", Valid: true},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}
	tokens, err := h.issueTokens(c, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue tokens"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"user":   toSafeUser(user),
		"tokens": tokens,
	})
}

type loginReq struct {
	Login    string `json:"login" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// @Summary Login
// @Tags auth
// @Accept json
// @Produce json
// @Param body body loginReq true "data logins"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user db.User
	var err error
	if strings.Contains(req.Login, "@") {
		user, err = h.q.GetUserByEmail(c, sql.NullString{String: req.Login, Valid: true})
	} else {
		user, err = h.q.GetUserByUsername(c, req.Login)
	}
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if !auth.CheckPassword(req.Password, user.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	_ = h.q.UpdateUserOnlineStatus(c, db.UpdateUserOnlineStatusParams{
		ID:       user.ID,
		IsOnline: sql.NullBool{Bool: true, Valid: true},
	})
	tokens, err := h.issueTokens(c, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue tokens"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user":   toSafeUser(user),
		"tokens": tokens,
	})
}

type refreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// @Summary Update token
// @Tags auth
// @Accept json
// @Produce json
// @Param body body refreshReq true "Refersh token"
// @Success 201 {object} tokenPair
// @Failure 400 {object} map[string]interface{}
// @Router /auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req refreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return

	}
	    log.Printf("🔍 refresh token: %s", req.RefreshToken[:20])

	claims, err := h.jwt.ParseRefresh(req.RefreshToken)
	if err != nil {
		log.Printf("❌ parse error: %v",err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}
	session,err := h.q.GetSessionByToken(c,req.RefreshToken)

    log.Printf("🔍 session: %+v, err: %v", session, err)

	if _, err := h.q.GetSessionByToken(c, req.RefreshToken); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "session expired or not found"})
		return

	}
	_ = h.q.DeleteSession(c, req.RefreshToken)

	tokens, err := h.issueTokens(c, claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue tokens"})
		return
	}
	c.JSON(http.StatusOK, tokens)
}

// @Summary Logout
// @Tags auth
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body refreshReq true "Refresh token"
// @Success 201 {object} map[string]interface{}
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	var req refreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = h.q.DeleteSession(c, req.RefreshToken)

	if uid, ok := c.Get("user_id"); ok {
		_ = h.q.UpdateUserOnlineStatus(c, db.UpdateUserOnlineStatusParams{
			ID:       uid.(uuid.UUID),
			IsOnline: sql.NullBool{Bool: false, Valid: true},
		})
	}
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

// @Summary current user
// @Tags auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} safeUser
// @Success 201 {object} map[string]interface{}
// @Router /auth/me [get]
func (h *AuthHandler) Me(c *gin.Context) {
	uid, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	user, err := h.q.GetUserByID(c, uid.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, toSafeUser(user))
}

type tokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func (h *AuthHandler) issueTokens(c *gin.Context, userID uuid.UUID) (*tokenPair, error) {
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

type safeUser struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email,omitempty"`
	AvatarURL string    `json:"avatar_url,omitempty"`
	Bio       string    `json:"bio,omitempty"`
	IsOnline  bool      `json:"is_online"`
	CreatedAt time.Time `json:"created_at"`
}

func toSafeUser(u db.User) safeUser {
	s := safeUser{
		ID:       u.ID,
		Username: u.Username,
		IsOnline: u.IsOnline.Bool,
	}
	if u.Email.Valid {
		s.Email = u.Email.String
	}
	if u.AvatarUrl.Valid {
		s.AvatarURL = u.AvatarUrl.String
	}
	if u.Bio.Valid {
		s.Bio = u.Bio.String
	}
	if u.CreatedAt.Valid {
		s.CreatedAt = u.CreatedAt.Time
	}
	return s
}

func parseInet(ipStr string) pqtype.Inet {
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		return pqtype.Inet{}
	}
	var ip []byte
	if addr.Is4() {
		a := addr.As4()
		ip = a[:]
	} else {
		a := addr.As16()
		ip = a[:]
	}
	_ = ip
	return pqtype.Inet{}
}

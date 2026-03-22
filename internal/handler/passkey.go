package handler

import (
	"encoding/json"
	"net/http"

	db "messenger/internal/db/sqlc"
	"messenger/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

type PasskeyHandler struct {
	queries  *db.Queries
	webauthn *webauthn.WebAuthn
}

func NewPasskeyHandler(queries *db.Queries, rpID, rpOrigin string) (*PasskeyHandler, error) {
	wconfig := &webauthn.Config{
		RPDisplayName: "Messenger",
		RPID:          rpID,               // например "localhost"
		RPOrigins:     []string{rpOrigin}, // например "http://localhost:3000"
	}
	w, err := webauthn.New(wconfig)
	if err != nil {
		return nil, err
	}
	return &PasskeyHandler{queries: queries, webauthn: w}, nil
}

// BEGIN REGISTRATION

// POST /api/v1/auth/passkey/register/begin
func (h *PasskeyHandler) BeginRegistration(c *gin.Context) {
	userID := middleware.GetUserID(c)

	// Получаем пользователя из БД
	// WebAuthn требует объект, реализующий интерфейс webauthn.User
	user, err := h.queries.GetUserByID(c, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found"})
		return
	}

	waUser := &webAuthnUser{
		id:          userID,
		name:        user.Username,
		displayName: user.Username,
	}

	options, sessionData, err := h.webauthn.BeginRegistration(waUser)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Сохраняем sessionData в cookie (или Redis — по желанию)
	sessionBytes, _ := json.Marshal(sessionData)
	c.SetCookie("passkey_session", string(sessionBytes), 300, "/", "", false, true)

	c.JSON(http.StatusOK, options)
}

// POST /api/v1/auth/passkey/register/finish
func (h *PasskeyHandler) FinishRegistration(c *gin.Context) {
	userID := middleware.GetUserID(c)

	user, err := h.queries.GetUserByID(c, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found"})
		return
	}

	waUser := &webAuthnUser{
		id:          userID,
		name:        user.Username,
		displayName: user.Username,
	}

	// Читаем sessionData из cookie
	sessionCookie, err := c.Cookie("passkey_session")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session not found"})
		return
	}

	var sessionData webauthn.SessionData
	if err := json.Unmarshal([]byte(sessionCookie), &sessionData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session"})
		return
	}

	credential, err := h.webauthn.FinishRegistration(waUser, sessionData, c.Request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Получаем имя passkey из тела запроса (опционально)
	var body struct {
		Name string `json:"name"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Name == "" {
		body.Name = "My Passkey"
	}

	_, err = h.queries.CreatePasskey(c, db.CreatePasskeyParams{
		UserID:       userID,
		CredentialID: string(credential.ID),
		PublicKey:    credential.PublicKey,
		Aaguid:       credential.Authenticator.AAGUID,
		SignCount:    int64(credential.Authenticator.SignCount),
		Name:         body.Name,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save passkey"})
		return
	}

	c.SetCookie("passkey_session", "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BEGIN LOGIN

// POST /api/v1/auth/passkey/login/begin
func (h *PasskeyHandler) BeginLogin(c *gin.Context) {
	var body struct {
		Username string `json:"username" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username required"})
		return
	}

	user, err := h.queries.GetUserByUsername(c, body.Username)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// Загружаем все passkeys пользователя
	passkeys, err := h.queries.GetPasskeysByUserID(c, user.ID)
	if err != nil || len(passkeys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no passkeys registered"})
		return
	}

	waUser := &webAuthnUser{
		id:          user.ID,
		name:        user.Username,
		displayName: user.Username,
		credentials: passkeyToCredentials(passkeys),
	}

	options, sessionData, err := h.webauthn.BeginLogin(waUser)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sessionBytes, _ := json.Marshal(sessionData)
	c.SetCookie("passkey_session", string(sessionBytes), 300, "/", "", false, true)

	c.JSON(http.StatusOK, options)
}

// POST /api/v1/auth/passkey/login/finish
func (h *PasskeyHandler) FinishLogin(c *gin.Context) {
	var body struct {
		Username string `json:"username" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username required"})
		return
	}

	user, err := h.queries.GetUserByUsername(c, body.Username)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	passkeys, err := h.queries.GetPasskeysByUserID(c, user.ID)
	if err != nil || len(passkeys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no passkeys registered"})
		return
	}

	waUser := &webAuthnUser{
		id:          user.ID,
		name:        user.Username,
		displayName: user.Username,
		credentials: passkeyToCredentials(passkeys),
	}

	sessionCookie, err := c.Cookie("passkey_session")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session not found"})
		return
	}

	var sessionData webauthn.SessionData
	if err := json.Unmarshal([]byte(sessionCookie), &sessionData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session"})
		return
	}

	credential, err := h.webauthn.FinishLogin(waUser, sessionData, c.Request)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// Обновляем sign_count
	_ = h.queries.UpdatePasskeySignCount(c, db.UpdatePasskeySignCountParams{
		CredentialID: string(credential.ID),
		SignCount:    int64(credential.Authenticator.SignCount),
	})

	c.SetCookie("passkey_session", "", -1, "/", "", false, true)

	// Выдаём JWT как обычно
	// TODO: вызвать твою функцию генерации токенов
	c.JSON(http.StatusOK, gin.H{"ok": true, "user_id": user.ID})
}

// GET  /api/v1/auth/passkey — список своих passkeys
func (h *PasskeyHandler) ListPasskeys(c *gin.Context) {
	userID := middleware.GetUserID(c)
	passkeys, err := h.queries.GetPasskeysByUserID(c, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	c.JSON(http.StatusOK, passkeys)
}

// DELETE /api/v1/auth/passkey/:id
func (h *PasskeyHandler) DeletePasskey(c *gin.Context) {
	userID := middleware.GetUserID(c)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.queries.DeletePasskey(c, db.DeletePasskeyParams{
		ID:     id,
		UserID: userID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

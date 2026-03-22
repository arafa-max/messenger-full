package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"messenger/internal/crypto"
	db "messenger/internal/db/sqlc"
)

type UploadKeysRequest struct {
	IdentityKey    string                 `json:"identity_key"`
	RegistrationID int                    `json:"registration_id"`
	SignedPreKey   SignedPreKeyRequest    `json:"signed_prekey"`
	OneTimePreKeys []OneTimePreKeyRequest `json:"one_time_prekeys"`
}

type SignedPreKeyRequest struct {
	KeyID     int    `json:"key_id"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
}

type OneTimePreKeyRequest struct {
	KeyID     int    `json:"key_id"`
	PublicKey string `json:"public_key"`
}

type GetKeyBundleResponse struct {
	IdentityKey   string                `json:"identity_key"`
	SignedPreKey  SignedPreKeyRequest   `json:"signed_prekey"`
	OneTimePreKey *OneTimePreKeyRequest `json:"one_time_prekey,omitempty"`
}

// UploadKeys — POST /api/v1/keys
// @Summary Загрузить E2EE ключи устройства
// @Description Сохраняет Identity Key, Signed PreKey и One-Time PreKeys
// @Tags E2EE
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body UploadKeysRequest true "Ключи устройства"
// @Success 200 {object} map[string]bool
// @Router /keys [post]
func UploadKeys(q *db.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var userUUID uuid.UUID
		if uid, ok := c.Get("user_id"); ok {
			userUUID, _ = uid.(uuid.UUID)
		}
		deviceNullUUID := parseNullUUID(c.GetString("device_id"))

		var req UploadKeysRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Сохраняем Identity Key
		_, err := q.SaveIdentityKey(c.Request.Context(), db.SaveIdentityKeyParams{
			UserID:         userUUID,
			DeviceID:       deviceNullUUID,
			PublicKey:      req.IdentityKey,
			RegistrationID: int32(req.RegistrationID),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "save identity key failed"})
			return
		}

		// Сохраняем Signed PreKey
		_, err = q.SaveSignedPreKey(c.Request.Context(), db.SaveSignedPreKeyParams{
			UserID:    userUUID,
			DeviceID:  deviceNullUUID,
			KeyID:     int32(req.SignedPreKey.KeyID),
			PublicKey: req.SignedPreKey.PublicKey,
			Signature: req.SignedPreKey.Signature,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "save signed prekey failed"})
			return
		}

		// Удаляем старые Signed PreKeys (оставляем только текущий)
		_ = q.DeleteOldSignedPreKeys(c.Request.Context(), db.DeleteOldSignedPreKeysParams{
			UserID:   userUUID,
			DeviceID: deviceNullUUID,
			KeyID:    int32(req.SignedPreKey.KeyID),
		})

		// Сохраняем One-Time PreKeys батчем
		for _, otpk := range req.OneTimePreKeys {
			err = q.SaveOneTimePreKey(c.Request.Context(), db.SaveOneTimePreKeyParams{
				UserID:    userUUID,
				DeviceID:  deviceNullUUID,
				KeyID:     int32(otpk.KeyID),
				PublicKey: otpk.PublicKey,
			})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "save one time prekey failed"})
				return
			}
		}

		// Сохраняем Key Bundle кэш
		// FIX: SaveKeyBundle ожидает uuid.UUID (не NullUUID) для device_id
		// Если устройство не указано — используем нулевой UUID
		if deviceNullUUID.Valid {
			signedPubBytes, _ := crypto.DecodeKey(req.SignedPreKey.PublicKey)
			sigBytes, _ := crypto.DecodeKey(req.SignedPreKey.Signature)
			bundle := crypto.KeyBundle{
				IdentityKey: []byte(req.IdentityKey),
				SignedPreKey: crypto.SignedPreKeyBundle{
					KeyID:     req.SignedPreKey.KeyID,
					PublicKey: signedPubBytes,
					Signature: sigBytes,
				},
			}
			bundleJSON, _ := json.Marshal(bundle)
			_, _ = q.SaveKeyBundle(c.Request.Context(), db.SaveKeyBundleParams{
				UserID:   userUUID,
				DeviceID: deviceNullUUID.UUID,
				Bundle:   bundleJSON,
			})
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// GetKeyBundle — GET /api/v1/keys/:user_id
// @Summary Получить PreKey Bundle пользователя
// @Description Возвращает ключи для инициализации E2EE сессии (X3DH)
// @Tags E2EE
// @Security BearerAuth
// @Produce json
// @Param user_id path string true "UUID пользователя"
// @Param device_id query string false "UUID устройства (опционально)"
// @Success 200 {object} GetKeyBundleResponse
// @Router /keys/{user_id} [get]
func GetKeyBundle(q *db.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		// FIX: user_id из path param, device_id из query param
		targetUUID := parseUUID(c.Param("user_id"))
		deviceID := c.Query("device_id") // опциональный query param
		deviceNullUUID := parseNullUUID(deviceID)

		// Identity Key
		identityKey, err := q.GetIdentityKey(c.Request.Context(), db.GetIdentityKeyParams{
			UserID:   targetUUID,
			DeviceID: deviceNullUUID,
		})
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "identity key not found"})
			return
		}

		// Signed PreKey
		signedPreKey, err := q.GetSignedPreKey(c.Request.Context(), db.GetSignedPreKeyParams{
			UserID:   targetUUID,
			DeviceID: deviceNullUUID,
		})
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "signed prekey not found"})
			return
		}

		resp := GetKeyBundleResponse{
			IdentityKey: identityKey.PublicKey,
			SignedPreKey: SignedPreKeyRequest{
				KeyID:     int(signedPreKey.KeyID),
				PublicKey: signedPreKey.PublicKey,
				Signature: signedPreKey.Signature,
			},
		}

		// One-Time PreKey — атомарно берём и помечаем как использованный
		otpk, err := q.GetAndUseOneTimePreKey(c.Request.Context(), db.GetAndUseOneTimePreKeyParams{
			UserID:   targetUUID,
			DeviceID: deviceNullUUID,
		})
		if err == nil {
			resp.OneTimePreKey = &OneTimePreKeyRequest{
				KeyID:     int(otpk.KeyID),
				PublicKey: otpk.PublicKey,
			}
		}
		// Если One-Time PreKey закончились — это не ошибка,
		// X3DH работает и без них (менее forward-secret, но работает)

		c.JSON(http.StatusOK, resp)
	}
}

// GetPreKeyCount — GET /api/v1/keys/count
// @Summary Количество доступных One-Time PreKeys
// @Description Клиент должен загружать новые ключи когда count < 10
// @Tags E2EE
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /keys/count [get]
func GetPreKeyCount(q *db.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		var userUUID uuid.UUID
		if uid, ok := c.Get("user_id"); ok {
			userUUID, _ = uid.(uuid.UUID)
		}

		count, err := q.CountAvailableOneTimePreKeys(c.Request.Context(), db.CountAvailableOneTimePreKeysParams{
			UserID:   userUUID,
			DeviceID: parseNullUUID(c.GetString("device_id")),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "count failed"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"count":      count,
			"needs_more": count < 10,
		})
	}
}

// GetCanary — GET /api/v1/canary
// @Summary Проверить Canary токен
// @Description Если alive=false — сервер мог быть скомпрометирован
// @Tags E2EE
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /canary [get]
func GetCanary(canary *crypto.Canary) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !crypto.IsCanaryAlive(canary) {
			c.JSON(http.StatusOK, gin.H{
				"alive":      false,
				"warning":    "CANARY EXPIRED — SERVER MAY BE COMPROMISED",
				"expires_at": canary.ExpiresAt,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"alive":      true,
			"token":      canary.Token,
			"statement":  canary.Statement,
			"issued_at":  canary.IssuedAt,
			"expires_at": canary.ExpiresAt,
		})
	}
}

func parseUUID(s string) uuid.UUID {
	u, _ := uuid.Parse(s)
	return u
}

func parseNullUUID(s string) uuid.NullUUID {
	u, err := uuid.Parse(s)
	if err != nil {
		return uuid.NullUUID{Valid: false}
	}
	return uuid.NullUUID{UUID: u, Valid: true}
}

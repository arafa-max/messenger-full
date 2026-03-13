package crypto

import (
	"crypto/rand"
	"fmt"
	"time"
)

// RecoverySession — сессия восстановления после компрометации
type RecoverySession struct {
	UserID        string `json:"user_id"`
	OldPublicKey  []byte `json:"old_public_key"`
	NewPublicKey  []byte `json:"new_public_key"`
	RecoveryCode  string `json:"recovery_code"`
	CreatedAt     int64  `json:"created_at"`
	CompletedAt   int64  `json:"completed_at,omitempty"`
	IsCompleted   bool   `json:"is_completed"`
}

// InitiateRecovery — начинаем процесс восстановления
// Вызывается когда пользователь сообщает о компрометации
func InitiateRecovery(userID string, oldIdentity *IdentityKeyPair) (*RecoverySession, *IdentityKeyPair, error) {
	// Генерируем новую Identity Key пару
	newIdentity, err := GenerateIdentityKeyPair()
	if err != nil {
		return nil, nil, fmt.Errorf("generate new identity: %w", err)
	}

	// Генерируем recovery code — для подтверждения через другое устройство
	codeBytes := make([]byte, 16)
	if _, err := rand.Read(codeBytes); err != nil {
		return nil, nil, fmt.Errorf("generate recovery code: %w", err)
	}
	recoveryCode := EncodeKey(codeBytes)

	session := &RecoverySession{
		UserID:       userID,
		OldPublicKey: oldIdentity.DHPublickey,
		NewPublicKey: newIdentity.DHPublickey,
		RecoveryCode: recoveryCode,
		CreatedAt:    time.Now().Unix(),
		IsCompleted:  false,
	}

	return session, newIdentity, nil
}

// CompleteRecovery — завершаем восстановление
// После этого все старые сессии инвалидируются
func CompleteRecovery(session *RecoverySession, recoveryCode string) error {
	if session.IsCompleted {
		return fmt.Errorf("recovery already completed")
	}
	if session.RecoveryCode != recoveryCode {
		return fmt.Errorf("invalid recovery code")
	}

	session.IsCompleted = true
	session.CompletedAt = time.Now().Unix()
	return nil
}

// ShouldRevokeSession — проверяем надо ли отозвать сессию
// Все сессии созданные до recovery — невалидны
func ShouldRevokeSession(sessionCreatedAt int64, recovery *RecoverySession) bool {
	if !recovery.IsCompleted {
		return false
	}
	// Сессия создана до завершения recovery → отзываем
	return sessionCreatedAt < recovery.CompletedAt
}
package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Canary — токен который доказывает что сервер не скомпрометирован
// Если канарейка исчезает или не обновляется — сервер под угрозой
type Canary struct {
	Token     string `json:"token"`      // публичный токен
	Hash      string `json:"hash"`       // хэш для верификации
	IssuedAt  int64  `json:"issued_at"`  // когда выпущен
	ExpiresAt int64  `json:"expires_at"` // когда истекает
	Statement string `json:"statement"`  // заявление сервера
}

// CanaryStatement — стандартное заявление
const CanaryStatement = "Мы не получали судебных запросов, ордеров или повесток. " +
	"Мы не были скомпрометированы. " +
	"Наши ключи шифрования не были переданы третьим лицам."

// GenerateCanary — генерируем новую канарейку (раз в неделю)
func GenerateCanary(secretKey []byte) (*Canary, error) {
	// Случайный токен
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate canary token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	now := time.Now().Unix()
	expiresAt := now + 7*24*60*60 // 7 дней

	// HMAC подпись — доказывает что канарейку выпустил именно сервер
	data := fmt.Sprintf("%s:%d:%d:%s", token, now, expiresAt, CanaryStatement)
	hash := computeCanaryHMAC(secretKey, data)

	return &Canary{
		Token:     token,
		Hash:      hash,
		IssuedAt:  now,
		ExpiresAt: expiresAt,
		Statement: CanaryStatement,
	}, nil
}

// VerifyCanary — проверяем канарейку
func VerifyCanary(canary *Canary, secretKey []byte) error {
	// Проверяем не истекла ли
	if time.Now().Unix() > canary.ExpiresAt {
		return fmt.Errorf("canary expired — server may be compromised")
	}

	// Проверяем HMAC
	data := fmt.Sprintf("%s:%d:%d:%s",
		canary.Token,
		canary.IssuedAt,
		canary.ExpiresAt,
		canary.Statement,
	)
	expected := computeCanaryHMAC(secretKey, data)
	if !hmac.Equal([]byte(expected), []byte(canary.Hash)) {
		return fmt.Errorf("canary signature invalid — canary may be tampered")
	}

	return nil
}

// IsCanaryAlive — простая проверка жива ли канарейка
func IsCanaryAlive(canary *Canary) bool {
	return time.Now().Unix() < canary.ExpiresAt
}

// computeCanaryHMAC — HMAC-SHA256 подпись
func computeCanaryHMAC(key []byte, data string) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}
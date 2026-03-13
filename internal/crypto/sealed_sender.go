package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

// SealedMessage — зашифрованное сообщение где сервер не знает отправителя
type SealedMessage struct {
	// Ephemeral публичный ключ отправителя (одноразовый)
	EphemeralPublic []byte `json:"eph"`
	// Зашифрованный сертификат отправителя + само сообщение
	Ciphertext []byte `json:"ct"`
}

// SenderCertificate — сертификат отправителя (шифруется внутри сообщения)
// Сервер НИКОГДА не видит эту структуру — она зашифрована ключом получателя
type SenderCertificate struct {
	SenderID        string `json:"sid"` // UUID отправителя
	SenderPublicKey []byte `json:"spk"` // публичный DH ключ отправителя
	Timestamp       int64  `json:"ts"`  // unix timestamp отправки
}

// SealMessage — Алиса запечатывает сообщение
//
// Что видит сервер:   EphemeralPublic (случайный), Ciphertext (непрозрачный)
// Что НЕ видит сервер: SenderID, SenderPublicKey, содержимое message
//
// Получатель узнаёт отправителя ТОЛЬКО после расшифровки.
func SealMessage(
	senderID string,
	senderIdentity *IdentityKeyPair,
	recipientPublic []byte, // публичный DH ключ получателя
	message []byte,
) (*SealedMessage, error) {
	// Генерируем ephemeral ключ — одноразовый, не связан с identity
	ephPriv := make([]byte, 32)
	if _, err := rand.Read(ephPriv); err != nil {
		return nil, fmt.Errorf("generate ephemeral: %w", err)
	}
	ephPub, err := curve25519.X25519(ephPriv, curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("ephemeral public: %w", err)
	}

	// DH между ephemeral и публичным ключом получателя
	dh, err := curve25519.X25519(ephPriv, recipientPublic)
	if err != nil {
		return nil, fmt.Errorf("dh: %w", err)
	}

	// Выводим ключ шифрования из DH + контекст
	encKey, err := deriveSealedKey(dh, ephPub, recipientPublic)
	if err != nil {
		return nil, err
	}

	// Создаём сертификат отправителя с реальным временем
	cert := SenderCertificate{
		SenderID:        senderID,
		SenderPublicKey: senderIdentity.DHPublickey,
		Timestamp:       time.Now().Unix(), // ← исправлено: реальное время
	}
	certBytes, err := json.Marshal(cert)
	if err != nil {
		return nil, fmt.Errorf("marshal cert: %w", err)
	}

	// Добавляем разделитель между сертификатом и сообщением
	// Используем длину сертификата как префикс (4 байта big-endian)
	// Это надёжнее чем парсить JSON на лету
	certLen := len(certBytes)
	payload := make([]byte, 4+certLen+len(message))
	payload[0] = byte(certLen >> 24)
	payload[1] = byte(certLen >> 16)
	payload[2] = byte(certLen >> 8)
	payload[3] = byte(certLen)
	copy(payload[4:], certBytes)
	copy(payload[4+certLen:], message)

	ciphertext, err := aesGCMEncrypt(encKey, payload)
	if err != nil {
		return nil, err
	}

	return &SealedMessage{
		EphemeralPublic: ephPub,
		Ciphertext:      ciphertext,
	}, nil
}

// UnsealMessage — Боб вскрывает сообщение и узнаёт отправителя
func UnsealMessage(
	recipientIdentity *IdentityKeyPair,
	sealed *SealedMessage,
) (*SenderCertificate, []byte, error) {
	// DH между приватным ключом получателя и ephemeral ключом отправителя
	dh, err := curve25519.X25519(recipientIdentity.DHPrivatekey, sealed.EphemeralPublic)
	if err != nil {
		return nil, nil, fmt.Errorf("dh: %w", err)
	}

	// Выводим тот же ключ что использовала Алиса
	encKey, err := deriveSealedKey(dh, sealed.EphemeralPublic, recipientIdentity.DHPublickey)
	if err != nil {
		return nil, nil, err
	}

	// Расшифровываем
	payload, err := aesGCMDecrypt(encKey, sealed.Ciphertext)
	if err != nil {
		return nil, nil, fmt.Errorf("decrypt sealed: %w", err)
	}

	// Разбираем payload: [4 байта длина сертификата][сертификат][сообщение]
	if len(payload) < 4 {
		return nil, nil, fmt.Errorf("payload too short")
	}
	certLen := int(payload[0])<<24 | int(payload[1])<<16 | int(payload[2])<<8 | int(payload[3])
	if len(payload) < 4+certLen {
		return nil, nil, fmt.Errorf("payload truncated: need %d cert bytes, have %d", certLen, len(payload)-4)
	}

	var cert SenderCertificate
	if err := json.Unmarshal(payload[4:4+certLen], &cert); err != nil {
		return nil, nil, fmt.Errorf("unmarshal cert: %w", err)
	}

	message := payload[4+certLen:]
	return &cert, message, nil
}

// deriveSealedKey — выводим ключ для Sealed Sender через HKDF
// Соль = ephemeral + recipient публичные ключи (контекст операции)
func deriveSealedKey(dh []byte, ephPub []byte, recipientPub []byte) ([]byte, error) {
	salt := append(ephPub, recipientPub...)
	info := []byte("messenger-sealed-v1")

	reader := hkdf.New(sha256.New, dh, salt, info)
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("derive sealed key: %w", err)
	}
	return key, nil
}
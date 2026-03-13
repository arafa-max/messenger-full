package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// KTLogEntry — запись в Key Transparency логе
// Как блокчейн — каждая запись ссылается на предыдущую
type KTLogEntry struct {
	UserID    string `json:"user_id"`
	PublicKey []byte `json:"public_key"`
	Timestamp int64  `json:"timestamp"`
	PrevHash  string `json:"prev_hash"` // хэш предыдущей записи
	Hash      string `json:"hash"`      // хэш этой записи
}

// KTLog — append-only лог ключей
type KTLog struct {
	Entries []*KTLogEntry
}

// NewKTLog — создаём новый лог
func NewKTLog() *KTLog {
	return &KTLog{
		Entries: make([]*KTLogEntry, 0),
	}
}

// AddEntry — добавляем новый ключ в лог
// Нельзя изменить предыдущие записи — только добавлять новые
func (l *KTLog) AddEntry(userID string, publicKey []byte) (*KTLogEntry, error) {
	// Хэш предыдущей записи
	prevHash := ""
	if len(l.Entries) > 0 {
		prevHash = l.Entries[len(l.Entries)-1].Hash
	}

	entry := &KTLogEntry{
		UserID:    userID,
		PublicKey: publicKey,
		Timestamp: time.Now().Unix(),
		PrevHash:  prevHash,
	}

	// Хэшируем запись
	entry.Hash = computeEntryHash(entry)
	l.Entries = append(l.Entries, entry)
	return entry, nil
}

// VerifyLog — проверяем что лог не был изменён
// Если кто-то изменил старую запись — хэши не совпадут
func (l *KTLog) VerifyLog() error {
	for i, entry := range l.Entries {
		// Проверяем хэш записи
		expected := computeEntryHash(entry)
		if entry.Hash != expected {
			return fmt.Errorf("log tampered at entry %d", i)
		}

		// Проверяем цепочку
		if i > 0 {
			if entry.PrevHash != l.Entries[i-1].Hash {
				return fmt.Errorf("chain broken at entry %d", i)
			}
		}
	}
	return nil
}

// GetLatestKey — получаем последний ключ пользователя
func (l *KTLog) GetLatestKey(userID string) (*KTLogEntry, error) {
	for i := len(l.Entries) - 1; i >= 0; i-- {
		if l.Entries[i].UserID == userID {
			return l.Entries[i], nil
		}
	}
	return nil, fmt.Errorf("no key found for user %s", userID)
}

// GetKeyHistory — вся история ключей пользователя
func (l *KTLog) GetKeyHistory(userID string) []*KTLogEntry {
	history := make([]*KTLogEntry, 0)
	for _, entry := range l.Entries {
		if entry.UserID == userID {
			history = append(history, entry)
		}
	}
	return history
}

// computeEntryHash — хэшируем запись
func computeEntryHash(entry *KTLogEntry) string {
	data := fmt.Sprintf("%s:%x:%d:%s",
		entry.UserID,
		entry.PublicKey,
		entry.Timestamp,
		entry.PrevHash,
	)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}
package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/pbkdf2"
)

const (
	backupVersion = 1
	pbkdf2Iter    = 600_000 // OWASP 2023 рекомендация для SHA-256
	pbkdf2KeyLen  = 32
	saltLen       = 32
)

// BackupHeader — заголовок зашифрованного бэкапа
// Хранится в открытом виде — нужен для расшифровки
type BackupHeader struct {
	Version   int    `json:"v"`
	Salt      []byte `json:"salt"` // соль для PBKDF2
	CreatedAt int64  `json:"ts"`   // время создания
	UserID    string `json:"uid"`  // чей бэкап
}

// EncryptedBackup — финальная структура бэкапа
type EncryptedBackup struct {
	Header     BackupHeader `json:"header"`
	Ciphertext []byte       `json:"ct"` // зашифрованные данные
}

// BackupData — что шифруем
type BackupData struct {
	Messages   []BackupMessage `json:"messages"`
	Chats      []BackupChat    `json:"chats"`
	ExportedAt int64           `json:"exported_at"`
}

type BackupMessage struct {
	ID        string `json:"id"`
	ChatID    string `json:"chat_id"`
	Content   string `json:"content"`
	SenderID  string `json:"sender_id"`
	CreatedAt int64  `json:"created_at"`
}

type BackupChat struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// CreateBackup — создаёт зашифрованный бэкап
//
// password — пользовательский пароль (никогда не покидает устройство)
// Ключ выводится через PBKDF2 — защита от brute-force
func CreateBackup(userID string, data *BackupData, password []byte) (*EncryptedBackup, error) {
	// Генерируем случайную соль
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	// Выводим ключ из пароля через PBKDF2-SHA256
	// 600k итераций — ~300ms на современном железе, brute-force нереален
	key := pbkdf2.Key(password, salt, pbkdf2Iter, pbkdf2KeyLen, sha256.New)

	// Сериализуем данные
	plaintext, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal backup data: %w", err)
	}

	// Шифруем AES-256-GCM
	ciphertext, err := aesGCMEncrypt(key, plaintext)
	if err != nil {
		return nil, fmt.Errorf("encrypt backup: %w", err)
	}

	header := BackupHeader{
		Version:   backupVersion,
		Salt:      salt,
		CreatedAt: time.Now().Unix(),
		UserID:    userID,
	}

	return &EncryptedBackup{
		Header:     header,
		Ciphertext: ciphertext,
	}, nil
}

// RestoreBackup — расшифровывает бэкап
func RestoreBackup(backup *EncryptedBackup, password []byte) (*BackupData, error) {
	if backup.Header.Version != backupVersion {
		return nil, fmt.Errorf("unsupported backup version: %d", backup.Header.Version)
	}

	// Восстанавливаем ключ из пароля + соли из заголовка
	key := pbkdf2.Key(password, backup.Header.Salt, pbkdf2Iter, pbkdf2KeyLen, sha256.New)

	// Расшифровываем
	plaintext, err := aesGCMDecrypt(key, backup.Ciphertext)
	if err != nil {
		// Намеренно не раскрываем причину — защита от оракула
		return nil, fmt.Errorf("decrypt failed: invalid password or corrupted backup")
	}

	var data BackupData
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return nil, fmt.Errorf("unmarshal backup: %w", err)
	}

	return &data, nil
}

// DeriveBackupKey — выводим ключ бэкапа из Identity Key пользователя
// Альтернатива паролю — для автоматических бэкапов без пароля
// Ключ привязан к identity — если identity скомпрометирован, бэкап тоже
func DeriveBackupKey(identityPrivKey []byte, userID string) ([]byte, error) {
	info := []byte("messenger-backup-v1:" + userID)
	reader := hkdf.New(sha256.New, identityPrivKey, nil, info)

	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("derive backup key: %w", err)
	}
	return key, nil
}

// EncryptWithKey — шифруем бэкап готовым ключом (без PBKDF2)
// Используется с DeriveBackupKey для автоматических бэкапов
func EncryptWithKey(userID string, data *BackupData, key []byte) (*EncryptedBackup, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
	}

	plaintext, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	ciphertext, err := aesGCMEncrypt(key, plaintext)
	if err != nil {
		return nil, fmt.Errorf("encrypt: %w", err)
	}

	return &EncryptedBackup{
		Header: BackupHeader{
			Version:   backupVersion,
			Salt:      nil, // нет соли — ключ передаётся напрямую
			CreatedAt: time.Now().Unix(),
			UserID:    userID,
		},
		Ciphertext: ciphertext,
	}, nil
}

// DecryptWithKey — расшифровываем готовым ключом
func DecryptWithKey(backup *EncryptedBackup, key []byte) (*BackupData, error) {
	plaintext, err := aesGCMDecrypt(key, backup.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt failed: invalid key or corrupted backup")
	}

	var data BackupData
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &data, nil
}

// SerializeBackup / DeserializeBackup — для сохранения на диск
func SerializeBackup(backup *EncryptedBackup) ([]byte, error) {
	return json.Marshal(backup)
}

func DeserializeBackup(data []byte) (*EncryptedBackup, error) {
	var backup EncryptedBackup
	if err := json.Unmarshal(data, &backup); err != nil {
		return nil, fmt.Errorf("deserialize backup: %w", err)
	}
	return &backup, nil
}

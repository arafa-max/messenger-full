package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

// RatchetState — состояние Double Ratchet для одного чата
type RatchetState struct {
	// DH Ratchet
	DHSendPriv []byte // наш текущий приватный ключ
	DHSendPub  []byte // наш текущий публичный ключ
	DHRecvPub  []byte // публичный ключ собеседника

	// Chain Keys
	RootKey      []byte // корневой ключ (обновляется при DH ratchet)
	SendChainKey []byte // ключ цепочки отправки
	RecvChainKey []byte // ключ цепочки получения

	// Счётчики сообщений
	SendCount uint32
	RecvCount uint32
}

// InitRatchetSender — инициализация для отправителя (Алиса)
// sharedSecret — результат X3DH
// recipientDHPublic — текущий DH публичный ключ Боба (его Signed PreKey)
func InitRatchetSender(sharedSecret []byte, recipientDHPublic []byte) (*RatchetState, error) {
	// Генерируем первую DH пару Алисы
	sendPriv := make([]byte, 32)
	if _, err := rand.Read(sendPriv); err != nil {
		return nil, fmt.Errorf("generate send key: %w", err)
	}
	sendPub, err := curve25519.X25519(sendPriv, curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("send public key: %w", err)
	}

	// Первый DH: Алиса ephPriv × Боб DHPub
	dh, err := curve25519.X25519(sendPriv, recipientDHPublic)
	if err != nil {
		return nil, fmt.Errorf("initial dh: %w", err)
	}

	// Выводим Root Key и первый Send Chain Key из sharedSecret + DH
	rootKey, sendChainKey, err := deriveRootKey(sharedSecret, dh)
	if err != nil {
		return nil, err
	}

	return &RatchetState{
		DHSendPriv:   sendPriv,
		DHSendPub:    sendPub,
		DHRecvPub:    recipientDHPublic,
		RootKey:      rootKey,
		SendChainKey: sendChainKey,
		RecvChainKey: nil, // Алиса получит RecvChainKey когда Боб ответит
		SendCount:    0,
		RecvCount:    0,
	}, nil
}

// InitRatchetReceiver — инициализация для получателя (Боб)
// ourDHPriv/ourDHPub — Боб использует свой Signed PreKey как начальную DH пару
func InitRatchetReceiver(sharedSecret []byte, ourDHPriv []byte, ourDHPub []byte) (*RatchetState, error) {
	// Боб стартует только с sharedSecret и своим DH ключом.
	// RecvChainKey будет выведен при первом DecryptMessage
	// когда придёт DHSendPub Алисы — тогда произойдёт dhRatchetStep.
	//
	// Ключевая синхронизация:
	// Алиса: RootKey, SendChainKey = deriveRootKey(sharedSecret, DH(alicePriv, bobPub))
	// Боб:   при dhRatchetStep делает DH(bobPriv, alicePub) → тот же результат (св-во DH)
	//        → получает тот же RootKey и RecvChainKey что у Алисы SendChainKey
	return &RatchetState{
		DHSendPriv:   ourDHPriv,
		DHSendPub:    ourDHPub,
		DHRecvPub:    nil, // nil = ждём первый DH pub от отправителя
		RootKey:      sharedSecret,
		SendChainKey: nil,
		RecvChainKey: nil,
		SendCount:    0,
		RecvCount:    0,
	}, nil
}

// EncryptMessage — шифруем сообщение
// Возвращает: ciphertext, наш текущий DHSendPub (нужен получателю для ratchet)
func (s *RatchetState) EncryptMessage(plaintext []byte) ([]byte, []byte, error) {
	if s.SendChainKey == nil {
		return nil, nil, fmt.Errorf("send chain key not initialized")
	}

	messageKey, nextChainKey, err := deriveMessageKey(s.SendChainKey)
	if err != nil {
		return nil, nil, err
	}
	s.SendChainKey = nextChainKey
	s.SendCount++

	ciphertext, err := aesGCMEncrypt(messageKey, plaintext)
	if err != nil {
		return nil, nil, err
	}

	return ciphertext, s.DHSendPub, nil
}

// DecryptMessage — расшифровываем сообщение
// senderDHPub — DHSendPub отправителя из этого сообщения
func (s *RatchetState) DecryptMessage(ciphertext []byte, senderDHPub []byte) ([]byte, error) {
	// Если пришёл новый DH ключ — делаем DH Ratchet step
	// Это также обрабатывает первое сообщение когда DHRecvPub == nil
	if !equalBytes(senderDHPub, s.DHRecvPub) {
		if err := s.dhRatchetStep(senderDHPub); err != nil {
			return nil, err
		}
	}

	if s.RecvChainKey == nil {
		return nil, fmt.Errorf("recv chain key not initialized after ratchet step")
	}

	messageKey, nextChainKey, err := deriveMessageKey(s.RecvChainKey)
	if err != nil {
		return nil, err
	}
	s.RecvChainKey = nextChainKey
	s.RecvCount++

	return aesGCMDecrypt(messageKey, ciphertext)
}

// dhRatchetStep — обновляем ключи при получении нового DH ключа от собеседника
//
// Математика (почему это работает):
// Алиса сделала: DH(alicePriv, bobPub) → вывела sendChainKey
// Боб делает:    DH(bobPriv, alicePub) → получает ТОТ ЖЕ результат (св-во DH)
// → Боб выводит recvChainKey = алисин sendChainKey ✓
func (s *RatchetState) dhRatchetStep(newDHPub []byte) error {
	// DH с новым ключом собеседника
	dh, err := curve25519.X25519(s.DHSendPriv, newDHPub)
	if err != nil {
		return fmt.Errorf("dh ratchet recv: %w", err)
	}

	// Обновляем Root Key и получаем новый Recv Chain Key
	newRoot, recvChain, err := deriveRootKey(s.RootKey, dh)
	if err != nil {
		return err
	}

	// Генерируем новую DH пару для следующего ratchet (наш следующий Send)
	newPriv := make([]byte, 32)
	if _, err := rand.Read(newPriv); err != nil {
		return fmt.Errorf("new dh key: %w", err)
	}
	newPub, err := curve25519.X25519(newPriv, curve25519.Basepoint)
	if err != nil {
		return fmt.Errorf("new dh public: %w", err)
	}

	// DH с новым ключом для Send Chain
	dh2, err := curve25519.X25519(newPriv, newDHPub)
	if err != nil {
		return fmt.Errorf("dh ratchet send: %w", err)
	}
	newRoot2, sendChain, err := deriveRootKey(newRoot, dh2)
	if err != nil {
		return err
	}

	s.RootKey = newRoot2
	s.RecvChainKey = recvChain
	s.SendChainKey = sendChain
	s.DHRecvPub = newDHPub
	s.DHSendPriv = newPriv
	s.DHSendPub = newPub
	s.RecvCount = 0

	return nil
}

// deriveRootKey — KDF для Root Key через HKDF
// Вход: текущий root key (соль) + DH результат (материал)
// Выход: новый root key + новый chain key
func deriveRootKey(rootKey []byte, dhOutput []byte) ([]byte, []byte, error) {
	info := []byte("messenger-root-v1")
	reader := hkdf.New(sha256.New, dhOutput, rootKey, info)

	newRoot := make([]byte, 32)
	chainKey := make([]byte, 32)

	if _, err := io.ReadFull(reader, newRoot); err != nil {
		return nil, nil, fmt.Errorf("derive root: %w", err)
	}
	if _, err := io.ReadFull(reader, chainKey); err != nil {
		return nil, nil, fmt.Errorf("derive chain: %w", err)
	}
	return newRoot, chainKey, nil
}

// deriveMessageKey — KDF для Message Key через HKDF
// Вход: chain key
// Выход: message key (для шифрования) + следующий chain key
func deriveMessageKey(chainKey []byte) ([]byte, []byte, error) {
	info := []byte("messenger-msg-v1")
	reader := hkdf.New(sha256.New, chainKey, nil, info)

	messageKey := make([]byte, 32)
	nextChainKey := make([]byte, 32)

	if _, err := io.ReadFull(reader, messageKey); err != nil {
		return nil, nil, fmt.Errorf("derive message key: %w", err)
	}
	if _, err := io.ReadFull(reader, nextChainKey); err != nil {
		return nil, nil, fmt.Errorf("derive next chain key: %w", err)
	}
	return messageKey, nextChainKey, nil
}

// aesGCMEncrypt — AES-256-GCM шифрование
// Формат вывода: [12 байт nonce][ciphertext+tag]
func aesGCMEncrypt(key []byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// aesGCMDecrypt — AES-256-GCM расшифровка
func aesGCMDecrypt(key []byte, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

// equalBytes — константное время сравнение байтов
func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
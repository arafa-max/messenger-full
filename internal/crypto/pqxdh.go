package crypto

import (
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/cloudflare/circl/kem/mlkem/mlkem768"
	"golang.org/x/crypto/hkdf"
)

// PQKeyPair — постквантовая пара ключей ML-KEM-768
type PQKeyPair struct {
	PublicKey  []byte
	PrivateKey []byte
}

// PQXDHSenderResult — результат PQXDH для отправителя
type PQXDHSenderResult struct {
	ClassicSecret  []byte // от X3DH
	PQSecret       []byte // от ML-KEM-768
	CombinedSecret []byte // финальный секрет
	Ciphertext     []byte // ML-KEM ciphertext (отправляем получателю)
}

// PQXDHReceiverResult — результат PQXDH для получателя
type PQXDHReceiverResult struct {
	CombinedSecret []byte
}

// GeneratePQKeyPair — генерирует ML-KEM-768 пару ключей
func GeneratePQKeyPair() (*PQKeyPair, error) {
	scheme := mlkem768.Scheme()

	pub, priv, err := scheme.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("generate ml-kem keypair: %w", err)
	}

	pubBytes, err := pub.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal public key: %w", err)
	}

	privBytes, err := priv.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}

	return &PQKeyPair{
		PublicKey:  pubBytes,
		PrivateKey: privBytes,
	}, nil
}

// PQXDHSender — Алиса: комбинируем X3DH + ML-KEM-768
func PQXDHSender(
	classicSecret []byte,        // результат обычного X3DH
	recipientPQPublicKey []byte, // ML-KEM публичный ключ Боба
) (*PQXDHSenderResult, error) {
	scheme := mlkem768.Scheme()

	// Восстанавливаем публичный ключ Боба
	bobPub, err := scheme.UnmarshalBinaryPublicKey(recipientPQPublicKey)
	if err != nil {
		return nil, fmt.Errorf("unmarshal pq public key: %w", err)
	}

	// ML-KEM Encapsulate — генерируем PQ секрет и ciphertext
	ciphertext, pqSecret, err := scheme.Encapsulate(bobPub)
	if err != nil {
		return nil, fmt.Errorf("ml-kem encapsulate: %w", err)
	}

	// Комбинируем классический + постквантовый секреты
	// ВАЖНО: соль детерминированная — производная от обоих секретов
	// Иначе отправитель и получатель получат разные combined секреты
	combined, err := combinePQSecrets(classicSecret, pqSecret)
	if err != nil {
		return nil, err
	}

	return &PQXDHSenderResult{
		ClassicSecret:  classicSecret,
		PQSecret:       pqSecret,
		CombinedSecret: combined,
		Ciphertext:     ciphertext,
	}, nil
}

// PQXDHReceiver — Боб: расшифровываем ML-KEM ciphertext
func PQXDHReceiver(
	classicSecret []byte,  // результат обычного X3DH
	ourPQPrivateKey []byte, // наш ML-KEM приватный ключ
	ciphertext []byte,      // ciphertext от Алисы
) (*PQXDHReceiverResult, error) {
	scheme := mlkem768.Scheme()

	// Восстанавливаем приватный ключ
	privKey, err := scheme.UnmarshalBinaryPrivateKey(ourPQPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("unmarshal pq private key: %w", err)
	}

	// ML-KEM Decapsulate — получаем тот же PQ секрет что и Алиса
	pqSecret, err := scheme.Decapsulate(privKey, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("ml-kem decapsulate: %w", err)
	}

	// Комбинируем — с той же детерминированной солью
	combined, err := combinePQSecrets(classicSecret, pqSecret)
	if err != nil {
		return nil, err
	}

	return &PQXDHReceiverResult{
		CombinedSecret: combined,
	}, nil
}

// combinePQSecrets — комбинируем X3DH + ML-KEM через HKDF
//
// Безопасность: гибридная схема — если ОДИН из алгоритмов взломан,
// общий секрет всё равно защищён вторым.
//
// Соль детерминированная: SHA-256(classicSecret || pqSecret)
// Это гарантирует что отправитель и получатель получат ОДИНАКОВЫЙ combined секрет.
// Случайная соль была бы ошибкой — её нужно было бы передавать отдельно.
func combinePQSecrets(classicSecret []byte, pqSecret []byte) ([]byte, error) {
	// Детерминированная соль = хэш от обоих секретов
	// Оба участника вычисляют одинаковую соль независимо
	saltInput := append(classicSecret, pqSecret...)
	saltHash := sha256.Sum256(saltInput)
	salt := saltHash[:]

	// Материал = конкатенация обоих секретов
	material := append(classicSecret, pqSecret...)

	info := []byte("messenger-pqxdh-v1")
	reader := hkdf.New(sha256.New, material, salt, info)

	combined := make([]byte, 32)
	if _, err := io.ReadFull(reader, combined); err != nil {
		return nil, fmt.Errorf("hkdf combine: %w", err)
	}
	return combined, nil
}
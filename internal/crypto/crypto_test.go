package crypto

import (
	"bytes"
	"testing"
	"time"
)

// ============================================================
// X3DH
// ============================================================

func TestX3DH_FullHandshake(t *testing.T) {
	// Генерируем ключи для Алисы и Боба
	alice, err := GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("alice identity: %v", err)
	}
	bob, err := GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("bob identity: %v", err)
	}

	bobSigned, err := GenerateSignedPreKey(bob, 1)
	if err != nil {
		t.Fatalf("bob signed prekey: %v", err)
	}

	bobOTPK, err := GenerateOneTimePreKeys(1, 1)
	if err != nil {
		t.Fatalf("bob one-time prekeys: %v", err)
	}

	// Алиса формирует bundle Боба
	bundle := &KeyBundle{
		IdentityKey: bob.DHPublickey,
		SignedPreKey: SignedPreKeyBundle{
			KeyID:     bobSigned.KeyID,
			PublicKey: bobSigned.PublicKey,
			Signature: bobSigned.Signature,
		},
		OneTimePreKey: &OneTimePreKeyBundle{
			KeyID:     bobOTPK[0].KeyID,
			PublicKey: bobOTPK[0].PublicKey,
		},
	}

	// Алиса выполняет X3DH
	aliceResult, err := X3DHSender(alice, bundle)
	if err != nil {
		t.Fatalf("alice x3dh sender: %v", err)
	}

	// Боб выполняет X3DH
	bobResult, err := X3DHReceiver(
		bob,
		bobSigned,
		bobOTPK[0],
		alice.DHPublickey,
		aliceResult.EphemeralKey,
	)
	if err != nil {
		t.Fatalf("bob x3dh receiver: %v", err)
	}

	// Секреты должны совпасть
	if !bytes.Equal(aliceResult.SharedSecret, bobResult.SharedSecret) {
		t.Errorf("shared secrets do not match!\nalice: %x\nbob:   %x",
			aliceResult.SharedSecret, bobResult.SharedSecret)
	}
}

func TestX3DH_WithoutOneTimePreKey(t *testing.T) {
	alice, _ := GenerateIdentityKeyPair()
	bob, _ := GenerateIdentityKeyPair()
	bobSigned, _ := GenerateSignedPreKey(bob, 1)

	// Без One-Time PreKey
	bundle := &KeyBundle{
		IdentityKey: bob.DHPublickey,
		SignedPreKey: SignedPreKeyBundle{
			KeyID:     bobSigned.KeyID,
			PublicKey: bobSigned.PublicKey,
			Signature: bobSigned.Signature,
		},
		OneTimePreKey: nil,
	}

	aliceResult, err := X3DHSender(alice, bundle)
	if err != nil {
		t.Fatalf("alice x3dh: %v", err)
	}

	bobResult, err := X3DHReceiver(bob, bobSigned, nil, alice.DHPublickey, aliceResult.EphemeralKey)
	if err != nil {
		t.Fatalf("bob x3dh: %v", err)
	}

	if !bytes.Equal(aliceResult.SharedSecret, bobResult.SharedSecret) {
		t.Error("secrets mismatch without one-time prekey")
	}
}

func TestX3DH_DifferentPartiesDontMatch(t *testing.T) {
	alice, _ := GenerateIdentityKeyPair()
	bob, _ := GenerateIdentityKeyPair()
	charlie, _ := GenerateIdentityKeyPair() // посторонний
	bobSigned, _ := GenerateSignedPreKey(bob, 1)

	bundle := &KeyBundle{
		IdentityKey: bob.DHPublickey,
		SignedPreKey: SignedPreKeyBundle{
			KeyID:     bobSigned.KeyID,
			PublicKey: bobSigned.PublicKey,
			Signature: bobSigned.Signature,
		},
	}

	aliceResult, _ := X3DHSender(alice, bundle)

	// Чарли пытается расшифровать — должен получить другой секрет
	charlieResult, _ := X3DHReceiver(charlie, bobSigned, nil, alice.DHPublickey, aliceResult.EphemeralKey)
	if charlieResult != nil && bytes.Equal(aliceResult.SharedSecret, charlieResult.SharedSecret) {
		t.Error("charlie should NOT get the same secret as alice and bob")
	}
}

// ============================================================
// PQXDH
// ============================================================

func TestPQXDH_FullHandshake(t *testing.T) {
	// Генерируем X3DH секрет (имитируем)
	classicSecret := make([]byte, 32)
	for i := range classicSecret {
		classicSecret[i] = byte(i)
	}

	// Боб генерирует PQ ключи
	bobPQ, err := GeneratePQKeyPair()
	if err != nil {
		t.Fatalf("generate pq keypair: %v", err)
	}

	// Алиса: encapsulate
	senderResult, err := PQXDHSender(classicSecret, bobPQ.PublicKey)
	if err != nil {
		t.Fatalf("pqxdh sender: %v", err)
	}

	// Боб: decapsulate
	receiverResult, err := PQXDHReceiver(classicSecret, bobPQ.PrivateKey, senderResult.Ciphertext)
	if err != nil {
		t.Fatalf("pqxdh receiver: %v", err)
	}

	// Комбинированные секреты должны совпасть
	if !bytes.Equal(senderResult.CombinedSecret, receiverResult.CombinedSecret) {
		t.Errorf("pqxdh combined secrets mismatch!\nsender:   %x\nreceiver: %x",
			senderResult.CombinedSecret, receiverResult.CombinedSecret)
	}
}

func TestPQXDH_DifferentClassicSecretsDontMatch(t *testing.T) {
	secret1 := make([]byte, 32)
	secret2 := make([]byte, 32)
	secret2[0] = 0xFF // отличается

	bobPQ, _ := GeneratePQKeyPair()
	senderResult, _ := PQXDHSender(secret1, bobPQ.PublicKey)

	// Боб использует другой classicSecret
	receiverResult, _ := PQXDHReceiver(secret2, bobPQ.PrivateKey, senderResult.Ciphertext)
	if receiverResult != nil && bytes.Equal(senderResult.CombinedSecret, receiverResult.CombinedSecret) {
		t.Error("different classic secrets must produce different combined secrets")
	}
}

// ============================================================
// Double Ratchet
// ============================================================

func TestRatchet_SendReceive(t *testing.T) {
	alice, _ := GenerateIdentityKeyPair()
	bob, _ := GenerateIdentityKeyPair()
	bobSigned, _ := GenerateSignedPreKey(bob, 1)

	bundle := &KeyBundle{
		IdentityKey: bob.DHPublickey,
		SignedPreKey: SignedPreKeyBundle{
			KeyID:     bobSigned.KeyID,
			PublicKey: bobSigned.PublicKey,
			Signature: bobSigned.Signature,
		},
	}

	// X3DH → shared secret
	aliceX3DH, _ := X3DHSender(alice, bundle)
	bobX3DH, _ := X3DHReceiver(bob, bobSigned, nil, alice.DHPublickey, aliceX3DH.EphemeralKey)

	// Инициализируем Ratchet
	aliceRatchet, err := InitRatchetSender(aliceX3DH.SharedSecret, bob.DHPublickey)
	if err != nil {
		t.Fatalf("init alice ratchet: %v", err)
	}
	// Боб инициализируется со своим DH приватным ключом
	// Его SendChainKey будет установлен после первого DecryptMessage (DH ratchet step)
	bobRatchet, err := InitRatchetReceiver(bobX3DH.SharedSecret, bob.DHPrivatekey, bob.DHPublickey)
	if err != nil {
		t.Fatalf("init bob ratchet: %v", err)
	}

	// Алиса отправляет сообщение
	plaintext := []byte("Привет, Боб!")
	ciphertext, aliceDHPub, err := aliceRatchet.EncryptMessage(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Боб расшифровывает — первый DH ratchet step произойдёт автоматически
	decrypted, err := bobRatchet.DecryptMessage(ciphertext, aliceDHPub)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("decrypted mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestRatchet_MultipleMessages(t *testing.T) {
	alice, _ := GenerateIdentityKeyPair()
	bob, _ := GenerateIdentityKeyPair()
	bobSigned, _ := GenerateSignedPreKey(bob, 1)

	bundle := &KeyBundle{
		IdentityKey: bob.DHPublickey,
		SignedPreKey: SignedPreKeyBundle{
			KeyID:     bobSigned.KeyID,
			PublicKey: bobSigned.PublicKey,
			Signature: bobSigned.Signature,
		},
	}

	aliceX3DH, _ := X3DHSender(alice, bundle)
	bobX3DH, _ := X3DHReceiver(bob, bobSigned, nil, alice.DHPublickey, aliceX3DH.EphemeralKey)

	aliceRatchet, _ := InitRatchetSender(aliceX3DH.SharedSecret, bob.DHPublickey)
	bobRatchet, _ := InitRatchetReceiver(bobX3DH.SharedSecret, bob.DHPrivatekey, bob.DHPublickey)

	messages := []string{
		"Первое сообщение",
		"Второе сообщение",
		"Третье сообщение",
	}

	for i, msg := range messages {
		ct, dhPub, err := aliceRatchet.EncryptMessage([]byte(msg))
		if err != nil {
			t.Fatalf("msg %d encrypt: %v", i, err)
		}
		dec, err := bobRatchet.DecryptMessage(ct, dhPub)
		if err != nil {
			t.Fatalf("msg %d decrypt: %v", i, err)
		}
		if string(dec) != msg {
			t.Errorf("msg %d: got %q, want %q", i, dec, msg)
		}
	}
}

// ============================================================
// Sealed Sender
// ============================================================

func TestSealedSender_RoundTrip(t *testing.T) {
	alice, err := GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("alice identity: %v", err)
	}
	bob, err := GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("bob identity: %v", err)
	}

	message := []byte("Секретное сообщение от Алисы")

	// Алиса запечатывает
	sealed, err := SealMessage("alice-uuid", alice, bob.DHPublickey, message)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	// Боб вскрывает
	cert, decrypted, err := UnsealMessage(bob, sealed)
	if err != nil {
		t.Fatalf("unseal: %v", err)
	}

	if cert.SenderID != "alice-uuid" {
		t.Errorf("sender id: got %q, want %q", cert.SenderID, "alice-uuid")
	}
	if !bytes.Equal(message, decrypted) {
		t.Errorf("message mismatch: got %q, want %q", decrypted, message)
	}

	// Timestamp должен быть реальным (не MaxInt64)
	now := time.Now().Unix()
	if cert.Timestamp > now || cert.Timestamp < now-5 {
		t.Errorf("timestamp looks wrong: %d (now: %d)", cert.Timestamp, now)
	}
}

func TestSealedSender_WrongRecipientCantDecrypt(t *testing.T) {
	alice, _ := GenerateIdentityKeyPair()
	bob, _ := GenerateIdentityKeyPair()
	charlie, _ := GenerateIdentityKeyPair()

	sealed, _ := SealMessage("alice-uuid", alice, bob.DHPublickey, []byte("for bob only"))

	// Чарли пытается расшифровать — должна быть ошибка
	_, _, err := UnsealMessage(charlie, sealed)
	if err == nil {
		t.Error("charlie should NOT be able to unseal bob's message")
	}
}

// ============================================================
// Backup
// ============================================================

func TestBackup_PasswordRoundTrip(t *testing.T) {
	password := []byte("super-secret-password-123")
	data := &BackupData{
		Messages: []BackupMessage{
			{ID: "msg1", ChatID: "chat1", Content: "Привет!", SenderID: "user1", CreatedAt: time.Now().Unix()},
		},
		Chats: []BackupChat{
			{ID: "chat1", Name: "Тест", Type: "dm"},
		},
		ExportedAt: time.Now().Unix(),
	}

	backup, err := CreateBackup("user1", data, password)
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}

	restored, err := RestoreBackup(backup, password)
	if err != nil {
		t.Fatalf("restore backup: %v", err)
	}

	if len(restored.Messages) != 1 || restored.Messages[0].Content != "Привет!" {
		t.Errorf("restored data mismatch: %+v", restored)
	}
}

func TestBackup_WrongPasswordFails(t *testing.T) {
	data := &BackupData{ExportedAt: time.Now().Unix()}
	backup, _ := CreateBackup("user1", data, []byte("correct-password"))

	_, err := RestoreBackup(backup, []byte("wrong-password"))
	if err == nil {
		t.Error("wrong password should fail to decrypt")
	}
}

func TestBackup_SerializeDeserialize(t *testing.T) {
	data := &BackupData{ExportedAt: time.Now().Unix()}
	backup, _ := CreateBackup("user1", data, []byte("password"))

	raw, err := SerializeBackup(backup)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	restored, err := DeserializeBackup(raw)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	if restored.Header.UserID != "user1" {
		t.Errorf("user id mismatch: %s", restored.Header.UserID)
	}
}

func TestBackup_DeriveKeyFromIdentity(t *testing.T) {
	identity, _ := GenerateIdentityKeyPair()
	data := &BackupData{ExportedAt: time.Now().Unix()}

	key, err := DeriveBackupKey(identity.DHPrivatekey, "user1")
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}

	backup, err := EncryptWithKey("user1", data, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	_, err = DecryptWithKey(backup, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
}

// ============================================================
// Canary
// ============================================================

func TestCanary_GenerateAndVerify(t *testing.T) {
	secret := []byte("server-secret-key")
	canary, err := GenerateCanary(secret)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if !IsCanaryAlive(canary) {
		t.Error("fresh canary should be alive")
	}

	if err := VerifyCanary(canary, secret); err != nil {
		t.Errorf("verify failed: %v", err)
	}
}

func TestCanary_TamperedTokenFails(t *testing.T) {
	secret := []byte("server-secret-key")
	canary, _ := GenerateCanary(secret)
	canary.Token = "tampered-token"

	if err := VerifyCanary(canary, secret); err == nil {
		t.Error("tampered canary should fail verification")
	}
}

func TestCanary_WrongKeyFails(t *testing.T) {
	canary, _ := GenerateCanary([]byte("original-key"))

	if err := VerifyCanary(canary, []byte("wrong-key")); err == nil {
		t.Error("wrong key should fail canary verification")
	}
}

// ============================================================
// Safety Numbers
// ============================================================

func TestSafetyNumbers_Deterministic(t *testing.T) {
	alice, _ := GenerateIdentityKeyPair()
	bob, _ := GenerateIdentityKeyPair()

	// Алиса вычисляет Safety Number
	num1 := SafetyNumber(alice.DHPublickey, bob.DHPublickey)
	// Боб вычисляет Safety Number
	num2 := SafetyNumber(alice.DHPublickey, bob.DHPublickey)

	if num1 != num2 {
		t.Errorf("safety numbers should be deterministic: %q != %q", num1, num2)
	}

	// Safety Number не должен быть пустым
	if num1 == "" {
		t.Error("safety number should not be empty")
	}
}

func TestSafetyNumbers_DifferentPairsAreDifferent(t *testing.T) {
	alice, _ := GenerateIdentityKeyPair()
	bob, _ := GenerateIdentityKeyPair()
	charlie, _ := GenerateIdentityKeyPair()

	numAliceBob := SafetyNumber(alice.DHPublickey, bob.DHPublickey)
	numAliceCharlie := SafetyNumber(alice.DHPublickey, charlie.DHPublickey)

	if numAliceBob == numAliceCharlie {
		t.Error("different pairs should produce different safety numbers")
	}
}

// ============================================================
// Key Transparency
// ============================================================

func TestKTLog_AddAndVerify(t *testing.T) {
	log := NewKTLog()
	identity, _ := GenerateIdentityKeyPair()

	_, err := log.AddEntry("user1", identity.DHPublickey)
	if err != nil {
		t.Fatalf("add entry: %v", err)
	}

	if err := log.VerifyLog(); err != nil {
		t.Errorf("verify log: %v", err)
	}
}

func TestKTLog_TamperedLogFails(t *testing.T) {
	log := NewKTLog()
	identity, _ := GenerateIdentityKeyPair()

	log.AddEntry("user1", identity.DHPublickey)

	// Портим запись
	log.Entries[0].UserID = "attacker"

	if err := log.VerifyLog(); err == nil {
		t.Error("tampered log should fail verification")
	}
}

func TestKTLog_GetLatestKey(t *testing.T) {
	log := NewKTLog()
	key1, _ := GenerateIdentityKeyPair()
	key2, _ := GenerateIdentityKeyPair()

	log.AddEntry("user1", key1.DHPublickey)
	log.AddEntry("user1", key2.DHPublickey) // ротация ключа

	latest, err := log.GetLatestKey("user1")
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}

	if !bytes.Equal(latest.PublicKey, key2.DHPublickey) {
		t.Error("should return the most recent key")
	}
}

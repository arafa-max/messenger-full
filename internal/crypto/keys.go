package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

type IdentityKeyPair struct {
	EdPublickey  ed25519.PublicKey
	EdPrivatekey ed25519.PrivateKey
	DHPublickey  []byte
	DHPrivatekey []byte
}

type SignedPreKeyPair struct {
	KeyID      int
	PublicKey  []byte
	PrivateKey []byte
	Signature  []byte
}

type OneTimePreKey struct {
	KeyID      int
	PublicKey  []byte
	PrivateKey []byte
}

type KeyBundle struct {
	IdentityKey   []byte
	SignedPreKey  SignedPreKeyBundle
	OneTimePreKey *OneTimePreKeyBundle
}

type SignedPreKeyBundle struct {
	KeyID     int
	PublicKey []byte
	Signature []byte
}
type OneTimePreKeyBundle struct {
	KeyID     int
	PublicKey []byte
}

// GenerateIdentityKeyPair
func GenerateIdentityKeyPair() (*IdentityKeyPair, error) {
	edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519: %w", err)
	}
	dhPriv := make([]byte, 32)
	if _, err := rand.Read(dhPriv); err != nil {
		return nil, fmt.Errorf("generate x25519 private: %w", err)
	}
	dhPub, err := curve25519.X25519(dhPriv, curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("generate x25519 private: %w", err)
	}

	return &IdentityKeyPair{
		EdPublickey:  edPub,
		EdPrivatekey: edPriv,
		DHPublickey:  dhPub,
		DHPrivatekey: dhPriv,
	}, nil
}

// GenerateSignedPreKey
func GenerateSignedPreKey(identity *IdentityKeyPair, keyID int) (*SignedPreKeyPair, error) {
	priv := make([]byte, 32)
	if _, err := rand.Read(priv); err != nil {
		return nil, fmt.Errorf("generate signed prekey: %w", err)
	}
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("generate signed prekey public: %w", err)
	}
	signature := ed25519.Sign(identity.EdPrivatekey, pub)
	return &SignedPreKeyPair{
		KeyID:      keyID,
		PublicKey:  pub,
		PrivateKey: priv,
		Signature:  signature,
	}, nil
}

// GenerateOneTimePreKeys
func GenerateOneTimePreKeys(count int, startID int) ([]*OneTimePreKey, error) {
	keys := make([]*OneTimePreKey, count)
	for i := 0; i < count; i++ {
		priv := make([]byte, 32)
		if _, err := rand.Read(priv); err != nil {
			return nil, fmt.Errorf("generate one time prekey: %d: %w", i, err)
		}
		pub, err := curve25519.X25519(priv, curve25519.Basepoint)
		if err != nil {
			return nil, fmt.Errorf("generate one time prekey public: %d: %w", i, err)
		}
		keys[i] = &OneTimePreKey{
			KeyID:      startID + i,
			PublicKey:  pub,
			PrivateKey: priv,
		}
	}
	return keys, nil
}

// VerifySignedPreKey
func VerifySignedPreKey(identityPublicKey ed25519.PublicKey, preKeyPublic []byte, signature []byte) bool {
	return ed25519.Verify(identityPublicKey, preKeyPublic, signature)
}

// SafetyNumber
func SafetyNumber(myIdentitiKey []byte, theirIdentity []byte) string {
	combined := append(myIdentitiKey, theirIdentity...)
	hash := sha256.Sum256(combined)
	result := ""
	for i := 0; i < 6; i++ {
		chuck := uint64(hash[i*5])<<32 | uint64(hash[i*5+1])<<24 |
			uint64(hash[i*5+2])<<16 | uint64(hash[i*5+3])<<8 | uint64(hash[i*5+4])
		result += fmt.Sprintf("%05d ", chuck%100000)
	}
	return result
}

func EncodeKey(key []byte) string {
	return base64.StdEncoding.EncodeToString(key)
}

func DecodeKey(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

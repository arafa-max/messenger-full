package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"

	"golang.org/x/crypto/curve25519"
)

type X3DHSenderResult struct {
	SharedSecret []byte
	EphemeralKey []byte
}
type X3DHReceiverResult struct {
	SharedSecret []byte
}

func X3DHSender(senderIdentity *IdentityKeyPair, bundle *KeyBundle) (*X3DHSenderResult, error) {
	// Generation ephemeral key
	ephPriv := make([]byte, 32)
	if _, err := rand.Read(ephPriv); err != nil {
		return nil, fmt.Errorf("generate ephemeral: %w", err)
	}
	ephPub, err := curve25519.X25519(ephPriv, curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("generate public: %w", err)
	}

	bobsigned := bundle.SignedPreKey.PublicKey

	// 4 DH operations
	dh1, err := curve25519.X25519(senderIdentity.DHPrivatekey, bobsigned)
	if err != nil {
		return nil, fmt.Errorf("dh1: %w", err)
	}

	dh2, err := curve25519.X25519(ephPriv, bundle.IdentityKey)
	if err != nil {
		return nil, fmt.Errorf("dh2: %w", err)
	}

	dh3, err := curve25519.X25519(ephPriv, bobsigned)
	if err != nil {
		return nil, fmt.Errorf("dh3: %w", err)
	}
	var dh4 []byte
	if bundle.OneTimePreKey != nil {
		dh4, err = curve25519.X25519(ephPriv, bundle.OneTimePreKey.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("dh4: %w", err)
		}
	}

	secret, err := deriveX3DHSecret(dh1, dh2, dh3, dh4)
	if err != nil {
		return nil, err
	}
	return &X3DHSenderResult{SharedSecret: secret, EphemeralKey: ephPub}, nil
}
func X3DHReceiver(
	receiverIdentity *IdentityKeyPair,
	signedPreKey *SignedPreKeyPair,
	oneTimePreKey *OneTimePreKey,
	senderIdentityPublic []byte,
	ephemeralPublic []byte,
) (*X3DHReceiverResult, error) {
	dh1, err := curve25519.X25519(signedPreKey.PrivateKey, senderIdentityPublic)
	if err != nil {
		return nil, fmt.Errorf("dh1: %w", err)
	}

	dh2, err := curve25519.X25519(receiverIdentity.DHPrivatekey, ephemeralPublic)
	if err != nil {
		return nil, fmt.Errorf("dh2: %w", err)
	}

	dh3, err := curve25519.X25519(signedPreKey.PrivateKey, ephemeralPublic)
	if err != nil {
		return nil, fmt.Errorf("dh3: %w", err)
	}
	var dh4 []byte
	if oneTimePreKey != nil {
		dh4, err = curve25519.X25519(oneTimePreKey.PrivateKey, ephemeralPublic)
		if err != nil {
			return nil, fmt.Errorf("dh4: %w", err)
		}
	}

	secret, err := deriveX3DHSecret(dh1, dh2, dh3, dh4)
	if err != nil {
		return nil, err
	}
	return &X3DHReceiverResult{SharedSecret: secret}, nil
}

func deriveX3DHSecret(dh1, dh2, dh3, dh4 []byte) ([]byte, error) {
	material := append(dh1, dh2...)
	material = append(material, dh3...)
	if dh4 != nil {
		material = append(material, dh4...)
	}
	salt := make([]byte, 32)
	info := []byte("messenger-x3dh-v1")
	reader := hkdf.New(sha256.New, material, salt, info)
	secret := make([]byte, 32)
	if _, err := io.ReadFull(reader, secret); err != nil {
		return nil, fmt.Errorf("hkdf: %w", err)
	}
	return secret, nil
}

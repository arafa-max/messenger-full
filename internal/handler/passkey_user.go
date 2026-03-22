package handler

import (
	db "messenger/internal/db/sqlc"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

type webAuthnUser struct {
	id          uuid.UUID
	name        string
	displayName string
	credentials []webauthn.Credential
}

func (u *webAuthnUser) WebAuthnID() []byte {
	return u.id[:]
}

func (u *webAuthnUser) WebAuthnName() string        { return u.name }
func (u *webAuthnUser) WebAuthnDisplayName() string { return u.displayName }
func (u *webAuthnUser) WebAuthnIcon() string        { return "" }
func (u *webAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

func passkeyToCredentials(rows []db.Passkey) []webauthn.Credential {
	creds := make([]webauthn.Credential, len(rows))
	for i, p := range rows {
		creds[i] = webauthn.Credential{
			ID:        []byte(p.CredentialID),
			PublicKey: p.PublicKey,
			Authenticator: webauthn.Authenticator{
				AAGUID:    p.Aaguid,
				SignCount: uint32(p.SignCount),
			},
		}
	}
	return creds
}
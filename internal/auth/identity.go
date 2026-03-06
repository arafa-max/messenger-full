package auth

import "github.com/google/uuid"

type User struct {
	ID       string
	Username string
}

func NewUser(username string) *User {
	return &User{
		ID:       uuid.New().String(),
		Username: username,
	}
}

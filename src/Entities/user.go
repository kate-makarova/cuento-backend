package Entities

import (
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	Id       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password,omitempty"` // Don't return password in JSON
	Roles    []Role `json:"roles"`
}

type ShortUser struct {
	Id       int    `json:"id"`
	Username string `json:"username"`
}

type UserProfile struct {
	UserId   int    `json:"user_id"`
	UserName string `json:"user_name"`
	Avatar   string `json:"avatar"`
}

func (u *User) HashPassword(password string) error {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	if err != nil {
		return err
	}
	u.Password = string(bytes)
	return nil
}

func (u *User) CheckPassword(providedPassword string) error {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(providedPassword))
}

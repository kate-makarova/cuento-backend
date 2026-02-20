package Middlewares

import "github.com/golang-jwt/jwt/v5"

// JwtKey is the secret key for signing JWT tokens.
// TODO: Load this from an environment variable in a real application.
var JwtKey = []byte("your_secret_key")

// Claims defines the structure of the JWT claims.
type Claims struct {
	Username string `json:"username"`
	UserID   int    `json:"user_id"`
	jwt.RegisteredClaims
}

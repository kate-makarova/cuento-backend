package Entities

import "time"

type AIChatMessage struct {
	Id          int       `json:"id"`
	UserId      int       `json:"user_id"`
	Role        string    `json:"role"` // "user" or "assistant"
	Content     string    `json:"content"`
	DateCreated time.Time `json:"date_created"`
}

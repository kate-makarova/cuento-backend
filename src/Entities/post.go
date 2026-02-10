package Entities

import "time"

type Post struct {
	Id               int               `json:"id"`
	TopicId          int               `json:"topic_id"`
	AuthorUserId     int               `json:"author_user_id"`
	DateCreated      time.Time         `json:"date_created"`
	Content          string            `json:"content"`
	CharacterProfile *CharacterProfile `json:"character_profile"`
}

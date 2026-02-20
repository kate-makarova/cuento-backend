package Entities

import "time"

type Post struct {
	Id                  int               `json:"id"`
	TopicId             int               `json:"topic_id"`
	AuthorUserId        int               `json:"author_user_id"`
	AuthorCharacterId   *int              `json:"author_character_id"`
	DateCreated         time.Time         `json:"date_created"`
	Content             string            `json:"content"`
	ContentHtml         string            `json:"content_html"`
	CharacterProfile    *CharacterProfile `json:"character_profile"`
	UserProfile         *UserProfile      `json:"user_profile"`
	UseCharacterProfile bool              `json:"use_character_profile"`
}

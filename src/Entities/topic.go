package Entities

import "time"

type TopicType int

const (
	General TopicType = 0
	Episode TopicType = 1
)

type Topic struct {
	Id           int       `json:"id"`
	Name         string    `json:"name"`
	Type         TopicType `json:"type"`
	DateCreated  time.Time `json:"date_created"`
	DateLastPost time.Time `json:"date_last_post"`
	PostNumber   int       `json:"post_number"`
	AuthorUserId int       `json:"author_user_id"`
}

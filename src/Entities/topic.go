package Entities

import "time"

type TopicType int

const (
	GeneralTopic        TopicType = 0
	EpisodeTopic        TopicType = 1
	CharacterSheetTopic TopicType = 2
)

type TopicStatus int

const (
	ActiveTopic   TopicStatus = 0
	InactiveTopic TopicStatus = 1
)

type Topic struct {
	Id                   int         `json:"id"`
	Status               TopicStatus `json:"status"`
	Name                 string      `json:"name"`
	Type                 TopicType   `json:"type"`
	DateCreated          time.Time   `json:"date_created"`
	DateLastPost         time.Time   `json:"date_last_post"`
	PostNumber           int         `json:"post_number"`
	AuthorUserId         int         `json:"author_user_id"`
	LastPostAuthorUserId int         `json:"last_post_author_user_id"`
}

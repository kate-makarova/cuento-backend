package Entities

type Character struct {
	Id              int               `json:"id"`
	UserId          int               `json:"user_id"`
	Name            string            `json:"name"`
	Avatar          *string           `json:"avatar"`
	CustomFields    CustomFieldEntity `json:"custom_fields"`
	CharacterStatus CharacterStatus   `json:"character_status"`
	TopicId         int               `json:"topic_id"`
	Factions        []Faction         `json:"factions"`
}

type ShortCharacter struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type CharacterStatus int

const (
	ActiveCharacter   CharacterStatus = 0
	InactiveCharacter CharacterStatus = 1
	PendingCharacter  CharacterStatus = 2
)

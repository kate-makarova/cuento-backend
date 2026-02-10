package Entities

type Episode struct {
	Id           int               `json:"id"`
	TopicId      int               `json:"topic_id"`
	Name         string            `json:"string"`
	CharacterIds []int             `json:"character_ids"`
	CustomFields CustomFieldEntity `json:"custom_fields"`
}

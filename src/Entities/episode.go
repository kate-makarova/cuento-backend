package Entities

type Episode struct {
	Id           int               `json:"id" db:"id"`
	Topic_Id     int               `json:"topic_id" db:"topic_id"`
	Name         string            `json:"name" db:"name"`
	CustomFields CustomFieldEntity `json:"custom_fields" db:"-"`
}

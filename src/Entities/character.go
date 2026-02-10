package Entities

type Character struct {
	Id           int               `json:"id"`
	UserId       int               `json:"user_id"`
	Name         string            `json:"name"`
	Avatar       *string           `json:"avatar"`
	CustomFields CustomFieldEntity `json:"custom_fields"`
}

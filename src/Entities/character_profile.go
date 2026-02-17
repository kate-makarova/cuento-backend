package Entities

type CharacterProfile struct {
	Id           int               `json:"id"`
	CharacterId  int               `json:"character_id"`
	Avatar       *string           `json:"avatar"`
	CustomFields CustomFieldEntity `json:"custom_fields"`
}

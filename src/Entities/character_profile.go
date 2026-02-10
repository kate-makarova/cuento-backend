package Entities

type CharacterProfile struct {
	Id           int               `json:"id"`
	CharacterId  int               `json:"character_id"`
	CustomFields CustomFieldEntity `json:"custom_fields"`
}

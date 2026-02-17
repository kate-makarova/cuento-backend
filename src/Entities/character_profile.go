package Entities

type CharacterProfile struct {
	Id            int               `json:"id"`
	CharacterId   int               `json:"character_id"`
	CharacterName string            `json:"character_name"`
	Avatar        *string           `json:"avatar"`
	CustomFields  CustomFieldEntity `json:"custom_fields"`
}

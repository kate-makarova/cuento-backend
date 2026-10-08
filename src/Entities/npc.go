package Entities

type NPC struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Avatar       *string `json:"avatar"`
	Description  *string `json:"description"`
	DisplayOrder int     `json:"display_order"`
}

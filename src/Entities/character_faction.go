package Entities

type CharacterFaction struct {
	Id                     int     `json:"id"`
	Name                   string  `json:"name"`
	ParentFaction          *int    `json:"parent_faction"`
	Image                  *string `json:"image"`
	Description            string  `json:"description"`
	IsUsedForCharacterList bool    `json:"is_used_for_character_list"`
}

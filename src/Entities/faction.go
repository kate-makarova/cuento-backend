package Entities

type Faction struct {
	Id            int           `json:"id"`
	Name          string        `json:"name"`
	ParentId      *int          `json:"parent_id"`
	Level         int           `json:"level"`
	Description   *string       `json:"description"`
	Icon          *string       `json:"icon"`
	ShowOnProfile bool          `json:"show_on_profile"`
	Characters    []Character   `json:"characters"`
	FactionStatus FactionStatus `json:"faction_status"`
}

type FactionStatus int

const (
	FactionActive   FactionStatus = 0
	FactionInactive FactionStatus = 1
	FactionPending  FactionStatus = 2
)

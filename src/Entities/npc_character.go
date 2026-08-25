package Entities

type NpcCharacter struct {
	Id           int               `json:"id"`
	Name         string            `json:"name"`
	Avatar       *string           `json:"avatar"`
	CampaignId   *int              `json:"campaign_id"`
	CustomFields CustomFieldEntity `json:"custom_fields" db:"-"`
}

func (n *NpcCharacter) GetBaseFields() []string {
	return []string{"name", "avatar", "campaign_id"}
}

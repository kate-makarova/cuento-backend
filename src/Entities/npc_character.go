package Entities

type NpcCharacterTopic struct {
	TopicId    int    `json:"topic_id"`
	TopicTitle string `json:"topic_title"`
	PostIds    []int  `json:"post_ids"`
}

type NpcCharacter struct {
	Id           int                 `json:"id"`
	Name         string              `json:"name"`
	Avatar       *string             `json:"avatar"`
	CampaignId   *int                `json:"campaign_id"`
	Topics       []NpcCharacterTopic `json:"topics,omitempty" db:"-"`
	CustomFields CustomFieldEntity   `json:"custom_fields" db:"-"`
}

func (n *NpcCharacter) GetBaseFields() []string {
	return []string{"name", "avatar", "campaign_id"}
}

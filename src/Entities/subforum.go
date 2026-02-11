package Entities

type Subform struct {
	Id          int    `json:"int"`
	CategoryId  int    `json:"category_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Position    int    `json:"position"`
	TopicNumber int    `json:"topic_number"`
	PostNumber  int    `json:"post_number"`
}

package Entities

type Subform struct {
	Id          int    `json:"int"`
	Name        string `json:"name"`
	Position    int    `json:"position"`
	TopicNumber int    `json:"topic_number"`
	PostNumber  int    `json:"post_number"`
}

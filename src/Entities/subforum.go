package Entities

type Subform struct {
	Id                 int     `json:"id"`
	CategoryId         int     `json:"category_id"`
	Name               string  `json:"name"`
	Description        string  `json:"description"`
	Position           int     `json:"position"`
	TopicNumber        int     `json:"topic_number"`
	PostNumber         int     `json:"post_number"`
	LastPostTopicId    *int    `json:"last_post_topic_id"`
	LastPostTopicName  *string `json:"last_post_topic_name"`
	LastPostId         *int    `json:"last_post_id"`
	DateLastPost       *string `json:"date_last_post"`
	LastPostAuthorName *string `json:"last_post_author_name"`
}

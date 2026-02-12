package Controllers

import (
	"cuento-backend/src/Entities"
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type ViewforumRow struct {
	Id                     int                `json:"id"`
	Name                   string             `json:"name"`
	Type                   Entities.TopicType `json:"type"`
	DateLastPost           time.Time          `json:"date_last_post"`
	PostNumber             int                `json:"post_number"`
	AuthorUserId           int                `json:"author_user_id"`
	AuthorUsername         string             `json:"author_username"`
	LastPostAuthorUserId   int                `json:"last_post_author_user_id"`
	LastPostAuthorUsername string             `json:"las_post_author_username"`
}

func GetTopicsBySubforum(c *gin.Context, db *sql.DB) {
	subforum := c.Param("subforum")
	pageStr := c.Param("page")
	page64, err := strconv.ParseInt(pageStr, 10, 0)
	page := int(page64)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page parameter must be a number"})
		return
	}

	var topics []ViewforumRow

	limit := 30
	rows, err := db.Query("SELECT topics.id, name, type, date_last_post, post_number, author_user_id, u.username as author_username, last_post_author_user_id, u2.username as las_post_author_username FROM topics JOIN cuento.users u on topics.author_user_id = u.id JOIN cuento.users u2 on topics.last_post_author_user_id = u2.id WHERE subforum_id = ? LIMIT ? OFFSET ?",
		subforum, limit, page*limit)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get topics"})
		return
	}

	defer rows.Close()

	for rows.Next() {
		var topic ViewforumRow
		err := rows.Scan(&topic.Id, &topic.Name, &topic.Type, &topic.DateLastPost, &topic.PostNumber, &topic.AuthorUserId, &topic.AuthorUsername, &topic.LastPostAuthorUserId, &topic.LastPostAuthorUsername)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan topics"})
			return
		}
		topics = append(topics, topic)
	}

	c.JSON(http.StatusOK, topics)
}

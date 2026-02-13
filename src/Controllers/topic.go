package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Events"
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type ViewforumRow struct {
	Id                     int                  `json:"id"`
	Status                 Entities.TopicStatus `json:"status"`
	Name                   string               `json:"name"`
	Type                   Entities.TopicType   `json:"type"`
	DateLastPost           *time.Time           `json:"date_last_post"`
	PostNumber             int                  `json:"post_number"`
	AuthorUserId           int                  `json:"author_user_id"`
	AuthorUsername         string               `json:"author_username"`
	LastPostAuthorUserId   int                  `json:"last_post_author_user_id"`
	LastPostAuthorUsername string               `json:"las_post_author_username"`
}

type CreateTopicRequest struct {
	SubforumId int    `json:"subforum_id" binding:"required"`
	Title      string `json:"title" binding:"required"`
	Content    string `json:"content" binding:"required"`
}

func GetTopicsBySubforum(c *gin.Context, db *sql.DB) {
	subforumStr := c.Param("subforum")
	pageStr := c.Param("pag")
	subforum64, err := strconv.ParseInt(subforumStr, 10, 0)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Incorrect subforum parameter"})
		return
	}
	subforum := int(subforum64)
	page64, err := strconv.ParseInt(pageStr, 10, 0)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Incorrect page parameter"})
		return
	}
	page := int(page64) - 1

	var topics []ViewforumRow

	limit := 30
	rows, err := db.Query("SELECT topics.id, status, name, type, date_last_post, post_number, author_user_id, u.username as author_username, last_post_author_user_id, u2.username as las_post_author_username FROM topics JOIN cuento.users u on topics.author_user_id = u.id JOIN cuento.users u2 on topics.last_post_author_user_id = u2.id WHERE subforum_id = ? LIMIT ? OFFSET ?",
		subforum, limit, page*limit)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get topics"})
		return
	}

	defer rows.Close()

	for rows.Next() {
		var topic ViewforumRow
		err := rows.Scan(&topic.Id, &topic.Status, &topic.Name, &topic.Type, &topic.DateLastPost, &topic.PostNumber, &topic.AuthorUserId, &topic.AuthorUsername, &topic.LastPostAuthorUserId, &topic.LastPostAuthorUsername)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan topics: " + err.Error()})
			return
		}
		topics = append(topics, topic)
	}

	c.JSON(http.StatusOK, topics)
}

func CreateTopic(c *gin.Context, db *sql.DB) {
	var req CreateTopicRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}

	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var userID int
	switch v := userIDVal.(type) {
	case int:
		userID = v
	case float64:
		userID = int(v)
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID type"})
		return
	}

	var username string
	err := db.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user details: " + err.Error()})
		return
	}

	tx, err := db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	// Insert Topic
	res, err := tx.Exec("INSERT INTO topics (subforum_id, name, author_user_id, date_created, date_last_post, status, type, post_number, last_post_author_user_id) VALUES (?, ?, ?, NOW(), NOW(), 0, 0, 1, ?)",
		req.SubforumId, req.Title, userID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert topic: " + err.Error()})
		return
	}
	topicID, err := res.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get topic ID"})
		return
	}

	// Insert Post
	res, err = tx.Exec("INSERT INTO posts (topic_id, author_user_id, content, date_created) VALUES (?, ?, ?, NOW())",
		topicID, userID, req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert post: " + err.Error()})
		return
	}
	postID, err := res.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get post ID"})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	// Publish event to update stats asynchronously
	Events.Publish(db, Events.TopicCreated, Events.TopicCreatedEvent{
		TopicID:    topicID,
		SubforumID: req.SubforumId,
		Title:      req.Title,
		PostID:     postID,
		UserID:     userID,
		Username:   username,
	})

	c.JSON(http.StatusCreated, gin.H{"message": "Topic created successfully", "topic_id": topicID})
}

package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Events"
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
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

type PostRow struct {
	Id             int       `json:"id"`
	AuthorUserId   int       `json:"author_user_id"`
	AuthorUsername string    `json:"author_username"`
	Content        string    `json:"content"`
	ContentHtml    string    `json:"content_html"`
	DatePosted     time.Time `json:"date_posted"`
}

func GetTopicsBySubforum(c *gin.Context, db *sql.DB) {
	subforumStr := c.Param("subforum")
	pageStr := c.Param("page")
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

	userID := Services.GetUserIdFromContext(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
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

func GetPostsByTopic(c *gin.Context, db *sql.DB) {
	topicIDStr := c.Param("id")
	pageStr := c.Param("page")

	topicID64, err := strconv.ParseInt(topicIDStr, 10, 0)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Incorrect topic ID"})
		return
	}
	topicID := int(topicID64)

	page64, err := strconv.ParseInt(pageStr, 10, 0)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Incorrect page parameter"})
		return
	}
	page := int(page64) - 1
	if page < 0 {
		page = 0
	}

	limit := 15
	rows, err := db.Query("SELECT p.id, p.author_user_id, u.username, p.content, p.date_created FROM posts p JOIN users u ON p.author_user_id = u.id WHERE p.topic_id = ? ORDER BY p.date_created ASC LIMIT ? OFFSET ?", topicID, limit, page*limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get posts: " + err.Error()})
		return
	}
	defer rows.Close()

	var posts []PostRow
	for rows.Next() {
		var post PostRow
		if err := rows.Scan(&post.Id, &post.AuthorUserId, &post.AuthorUsername, &post.Content, &post.DatePosted); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan posts: " + err.Error()})
			return
		}
		post.ContentHtml = Entities.ParseBBCode(post.Content)
		posts = append(posts, post)
	}

	c.JSON(http.StatusOK, posts)
}

func GetTopic(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid ID"})
		c.Abort()
		return
	}

	var topic Entities.Topic
	query := "SELECT t.id, t.status, t.name, t.type, t.date_created, t.date_last_post, t.post_number, t.author_user_id, u.username, t.last_post_author_user_id, u2.username, t.subforum_id FROM topics t JOIN users u ON t.author_user_id = u.id LEFT JOIN users u2 ON t.last_post_author_user_id = u2.id WHERE t.id = ?"
	err = db.QueryRow(query, id).Scan(
		&topic.Id,
		&topic.Status,
		&topic.Name,
		&topic.Type,
		&topic.DateCreated,
		&topic.DateLastPost,
		&topic.PostNumber,
		&topic.AuthorUserId,
		&topic.AuthorUsername,
		&topic.LastPostAuthorUserId,
		&topic.LastPostAuthorName,
		&topic.SubforumId,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "Topic not found"})
		} else {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get topic: " + err.Error()})
		}
		c.Abort()
		return
	}

	if topic.Type == Entities.EpisodeTopic {
		var episodeID int
		err := db.QueryRow("SELECT id FROM episode_base WHERE topic_id = ?", topic.Id).Scan(&episodeID)
		if err != nil {
			if err != sql.ErrNoRows {
				_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get episode ID for topic: " + err.Error()})
				c.Abort()
			}
			c.JSON(http.StatusOK, topic)
			return
		}

		entity, err := Services.GetEntity(int64(episodeID), "episode", db)
		if err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get episode entity: " + err.Error()})
			c.Abort()
			return
		}

		if episode, ok := entity.(*Entities.Episode); ok {
			// Fetch characters for the episode
			charRows, err := db.Query("SELECT cb.id, cb.name FROM character_base cb JOIN episode_character ec ON cb.id = ec.character_id WHERE ec.episode_id = ?", episode.Id)
			if err == nil {
				var characters []*Entities.ShortCharacter
				for charRows.Next() {
					var char Entities.ShortCharacter
					if err := charRows.Scan(&char.Id, &char.Name); err == nil {
						characters = append(characters, &char)
					}
				}
				episode.Characters = characters
				charRows.Close()
			}
			topic.Episode = episode
		}
	}

	c.JSON(http.StatusOK, topic)
}

package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Events"
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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

type CreatePostRequest struct {
	TopicID             int    `json:"topic_id" binding:"required"`
	Content             string `json:"content" binding:"required"`
	UseCharacterProfile bool   `json:"use_character_profile"`
	CharacterProfileID  *int   `json:"character_profile_id"`
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

	topicID, err := strconv.Atoi(topicIDStr)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid topic ID"})
		c.Abort()
		return
	}

	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}

	limit := 15
	offset := (page - 1) * limit

	// 1. Get custom field columns from the config table
	var configJSON string
	err = db.QueryRow("SELECT config FROM custom_field_config WHERE entity_type = 'character_profile'").Scan(&configJSON)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get character profile config: " + err.Error()})
		c.Abort()
		return
	}

	var customConfig []Entities.CustomFieldConfig
	if err := json.Unmarshal([]byte(configJSON), &customConfig); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to parse character profile config: " + err.Error()})
		c.Abort()
		return
	}

	var flattenedCols []string
	for _, field := range customConfig {
		flattenedCols = append(flattenedCols, "cpf."+field.MachineFieldName)
	}

	// 2. Construct the main query
	query := fmt.Sprintf(`
		SELECT
			p.id, p.author_user_id, p.date_created, p.content, p.use_character_profile,
			u.username, u.avatar,
			cp.id as character_profile_id, cp.character_id, cb.name as character_name, cp.avatar as character_avatar,
			%s
		FROM posts p
		LEFT JOIN users u ON p.author_user_id = u.id
		LEFT JOIN character_profile_base cp ON p.character_profile_id = cp.id
		LEFT JOIN character_base cb ON cp.character_id = cb.id
		LEFT JOIN character_profile_flattened cpf ON cp.id = cpf.entity_id
		WHERE p.topic_id = ?
		ORDER BY p.date_created ASC
		LIMIT ? OFFSET ?
	`, strings.Join(flattenedCols, ", "))

	rows, err := db.Query(query, topicID, limit, offset)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get posts: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	// 3. Scan and process results
	cols, _ := rows.Columns()
	posts := make([]Entities.Post, 0) // Initialize slice

	for rows.Next() {
		// Scan into a map
		values := make([]interface{}, len(cols))
		for i := range values {
			values[i] = new(sql.RawBytes)
		}
		if err := rows.Scan(values...); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan post data: " + err.Error()})
			c.Abort()
			return
		}

		rowMap := make(map[string]interface{})
		for i, colName := range cols {
			val := *(values[i].(*sql.RawBytes))
			if val != nil {
				rowMap[colName] = string(val) // Store as string for now
			}
		}

		// Populate Post struct from map
		var post Entities.Post
		post.Id, _ = strconv.Atoi(rowMap["id"].(string))
		post.AuthorUserId, _ = strconv.Atoi(rowMap["author_user_id"].(string))
		post.DateCreated, _ = time.Parse("2006-01-02 15:04:05", rowMap["date_created"].(string))
		post.Content = rowMap["content"].(string)
		post.UseCharacterProfile, _ = strconv.ParseBool(rowMap["use_character_profile"].(string))

		if post.UseCharacterProfile {
			var charProfile Entities.CharacterProfile
			if id, ok := rowMap["character_profile_id"]; ok {
				charProfile.Id, _ = strconv.Atoi(id.(string))
			}
			if id, ok := rowMap["character_id"]; ok {
				charProfile.CharacterId, _ = strconv.Atoi(id.(string))
			}
			if name, ok := rowMap["character_name"]; ok {
				charProfile.CharacterName = name.(string)
			}
			if avatar, ok := rowMap["character_avatar"]; ok {
				avatarStr := avatar.(string)
				charProfile.Avatar = &avatarStr
			}

			// Populate custom fields
			customFields := make(map[string]interface{})
			for _, field := range customConfig {
				if val, ok := rowMap[field.MachineFieldName]; ok {
					customFields[field.MachineFieldName] = val
				}
			}
			charProfile.CustomFields.CustomFields = customFields
			charProfile.CustomFields.FieldConfig = customConfig // Add this line
			post.CharacterProfile = &charProfile
		} else {
			var userProfile Entities.UserProfile
			userProfile.UserId = post.AuthorUserId
			if username, ok := rowMap["username"]; ok {
				userProfile.UserName = username.(string)
			}
			if avatar, ok := rowMap["avatar"]; ok {
				userProfile.Avatar = avatar.(string)
			}
			post.UserProfile = &userProfile
		}
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

func CreatePost(c *gin.Context, db *sql.DB) {
	var req CreatePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}

	userID := Services.GetUserIdFromContext(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	tx, err := db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	// Insert Post
	res, err := tx.Exec("INSERT INTO posts (topic_id, author_user_id, content, date_created, use_character_profile, character_profile_id) VALUES (?, ?, ?, NOW(), ?, ?)",
		req.TopicID, userID, req.Content, req.UseCharacterProfile, req.CharacterProfileID)
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

	c.JSON(http.StatusCreated, gin.H{"message": "Post created successfully", "post_id": postID})
}

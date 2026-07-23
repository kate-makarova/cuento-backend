package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	UserDataProcessingStatusPending   = 0
	UserDataProcessingStatusProcessed = 1
	UserDataProcessingStatusPublished = 2
	ForumTypeMyBB                     = 1
)

type UserDataProcessingResponse struct {
	Id          int       `json:"id"`
	DateCreated time.Time `json:"date_created"`
	UserId      int       `json:"user_id"`
	Status      int       `json:"status"`
}

type MybbPostEntry struct {
	Id       string `json:"id"`
	Username string `json:"username"`
	UserId   string `json:"user_id"`
	Message  string `json:"message"`
	Posted   string `json:"posted"`
	TopicId  string `json:"topic_id"`
	ForumId  string `json:"forum_id"`
	Subject  string `json:"subject"`
}

type ProcessMybbTopicRequest struct {
	ProcessingId int             `json:"processing_id" binding:"required"`
	ForumDomain  string          `json:"forum_domain" binding:"required"`
	Response     []MybbPostEntry `json:"response" binding:"required"`
}

type MigrationFoundUser struct {
	UserId   string `json:"user_id"`
	UserName string `json:"user_name"`
}

type ProcessingResultResponse struct {
	FoundPosts        int                  `json:"found_posts"`
	SkippedDuplicates int                  `json:"skipped_duplicates"`
	AllParsed         bool                 `json:"all_parsed"`
	FoundUsers        []MigrationFoundUser `json:"found_users"`
}

type PublishUserDataProcessingRequest struct {
	ProcessingId int            `json:"processing_id" binding:"required"`
	TopicId      int            `json:"topic_id" binding:"required"`
	UserMap      map[string]int `json:"user_map" binding:"required"`
}

func CreateUserDataProcessing(c *gin.Context, db *sql.DB) {
	userID := Services.GetUserIdFromContext(c)
	if userID == 0 {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusUnauthorized, Message: "Unauthorized"})
		c.Abort()
		return
	}

	now := time.Now()
	res, err := db.Exec(
		"INSERT INTO user_data_processing (date_created, user_id, status) VALUES (?, ?, ?)",
		now, userID, UserDataProcessingStatusPending,
	)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to create processing: " + err.Error()})
		c.Abort()
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get processing ID: " + err.Error()})
		c.Abort()
		return
	}

	c.JSON(http.StatusCreated, UserDataProcessingResponse{
		Id:          int(id),
		DateCreated: now,
		UserId:      userID,
		Status:      UserDataProcessingStatusPending,
	})
}

func ProcessMybbTopicJson(c *gin.Context, db *sql.DB) {
	userID := Services.GetUserIdFromContext(c)
	if userID == 0 {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusUnauthorized, Message: "Unauthorized"})
		c.Abort()
		return
	}

	var req ProcessMybbTopicRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	var ownerID int
	err := db.QueryRow("SELECT user_id FROM user_data_processing WHERE id = ?", req.ProcessingId).Scan(&ownerID)
	if err == sql.ErrNoRows {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "Processing record not found"})
		c.Abort()
		return
	}
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to verify processing: " + err.Error()})
		c.Abort()
		return
	}
	if ownerID != userID {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusForbidden, Message: "Access denied"})
		c.Abort()
		return
	}

	now := time.Now()
	usersSeen := map[string]string{}
	savedCount := 0
	skippedCount := 0

	for _, post := range req.Response {
		var existing int
		_ = db.QueryRow(
			"SELECT COUNT(*) FROM user_data_migration WHERE processing_id = ? AND original_topic_id = ? AND original_post_id = ?",
			req.ProcessingId, post.TopicId, post.Id,
		).Scan(&existing)
		if existing > 0 {
			skippedCount++
			usersSeen[post.UserId] = post.Username
			continue
		}

		bbParsed := Services.ParseMybbHTML(post.Message)
		_, err := db.Exec(`
			INSERT INTO user_data_migration
				(original_forum_domain, original_forum_type, original_topic_id, original_post_id,
				 original_user_id, original_user_name, original_post_content_html, post_content_bb_parsed,
				 is_published, date_processed, processing_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
			req.ForumDomain, ForumTypeMyBB, post.TopicId, post.Id,
			post.UserId, post.Username, post.Message, bbParsed,
			now, req.ProcessingId,
		)
		if err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to save post " + post.Id + ": " + err.Error()})
			c.Abort()
			return
		}
		usersSeen[post.UserId] = post.Username
		savedCount++
	}

	_, _ = db.Exec("UPDATE user_data_processing SET status = ? WHERE id = ?", UserDataProcessingStatusProcessed, req.ProcessingId)

	foundUsers := make([]MigrationFoundUser, 0, len(usersSeen))
	for uid, uname := range usersSeen {
		foundUsers = append(foundUsers, MigrationFoundUser{UserId: uid, UserName: uname})
	}

	c.JSON(http.StatusOK, ProcessingResultResponse{
		FoundPosts:        savedCount,
		SkippedDuplicates: skippedCount,
		AllParsed:         true,
		FoundUsers:        foundUsers,
	})
}

func UserDataProcessingPublish(c *gin.Context, db *sql.DB) {
	userID := Services.GetUserIdFromContext(c)
	if userID == 0 {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusUnauthorized, Message: "Unauthorized"})
		c.Abort()
		return
	}

	var req PublishUserDataProcessingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	// Verify the processing record belongs to this user
	var ownerID int
	err := db.QueryRow("SELECT user_id FROM user_data_processing WHERE id = ?", req.ProcessingId).Scan(&ownerID)
	if err == sql.ErrNoRows {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "Processing record not found"})
		c.Abort()
		return
	}
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to verify processing: " + err.Error()})
		c.Abort()
		return
	}
	if ownerID != userID {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusForbidden, Message: "Access denied"})
		c.Abort()
		return
	}

	// Verify target topic exists and get its subforum_id, type, and name
	var subforumID int
	var topicType Entities.TopicType
	var topicName string
	err = db.QueryRow("SELECT COALESCE(subforum_id, 0), type, name FROM topics WHERE id = ?", req.TopicId).Scan(&subforumID, &topicType, &topicName)
	if err == sql.ErrNoRows {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "Target topic not found"})
		c.Abort()
		return
	}
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to verify topic: " + err.Error()})
		c.Abort()
		return
	}

	// Pre-resolve every unique character_id -> user_id so we can validate upfront
	userIdByCharacter := make(map[int]int)
	for _, charID := range req.UserMap {
		if _, resolved := userIdByCharacter[charID]; resolved {
			continue
		}
		var charUserID int
		err := db.QueryRow("SELECT user_id FROM character_base WHERE id = ?", charID).Scan(&charUserID)
		if err == sql.ErrNoRows {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: fmt.Sprintf("Character %d not found", charID)})
			c.Abort()
			return
		}
		if err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: fmt.Sprintf("Failed to resolve character %d: %s", charID, err.Error())})
			c.Abort()
			return
		}
		userIdByCharacter[charID] = charUserID
	}

	// Fetch all unpublished rows for this processing in original post order
	rows, err := db.Query(`
		SELECT id, original_user_id, COALESCE(post_content_bb_parsed, original_post_content_html, '')
		FROM user_data_migration
		WHERE processing_id = ? AND is_published = 0
		ORDER BY CAST(original_post_id AS UNSIGNED) ASC`,
		req.ProcessingId,
	)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to fetch migration rows: " + err.Error()})
		c.Abort()
		return
	}

	type pendingRow struct {
		migrationId    int
		originalUserId string
		content        string
	}
	var pending []pendingRow
	for rows.Next() {
		var r pendingRow
		if err := rows.Scan(&r.migrationId, &r.originalUserId, &r.content); err != nil {
			rows.Close()
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan migration row: " + err.Error()})
			c.Abort()
			return
		}
		pending = append(pending, r)
	}
	rows.Close()

	publishedCount := 0
	skippedCount := 0

	for _, row := range pending {
		charID, mapped := req.UserMap[row.originalUserId]
		if !mapped {
			skippedCount++
			continue
		}
		authorUserID := userIdByCharacter[charID]

		tx, err := db.Begin()
		if err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to start transaction: " + err.Error()})
			c.Abort()
			return
		}

		res, err := tx.Exec(
			"INSERT INTO posts (topic_id, author_user_id, content, date_created, use_character_profile, character_profile_id) VALUES (?, ?, ?, NOW(), false, NULL)",
			req.TopicId, authorUserID, row.content,
		)
		if err != nil {
			tx.Rollback()
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to insert post: " + err.Error()})
			c.Abort()
			return
		}

		postID, err := res.LastInsertId()
		if err != nil {
			tx.Rollback()
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get post ID: " + err.Error()})
			c.Abort()
			return
		}

		_, err = tx.Exec(`
			UPDATE user_data_migration
			SET user_id = ?, character_id = ?, topic_id = ?, post_id = ?, is_published = 1, date_published = NOW()
			WHERE id = ?`,
			authorUserID, charID, req.TopicId, postID, row.migrationId,
		)
		if err != nil {
			tx.Rollback()
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to update migration record: " + err.Error()})
			c.Abort()
			return
		}

		if err := tx.Commit(); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to commit: " + err.Error()})
			c.Abort()
			return
		}

		// Update stats directly — no notifications, no currency, no websocket broadcast.

		// Global post counter
		_, _ = db.Exec("UPDATE global_stats SET stat_value = stat_value + 1 WHERE stat_name = 'total_post_number'")
		if topicType == Entities.EpisodeTopic {
			_, _ = db.Exec("UPDATE global_stats SET stat_value = stat_value + 1 WHERE stat_name = 'total_episode_post_number'")
		}

		// Topic post count, last-post metadata
		_, _ = db.Exec(`
			UPDATE topics
			SET post_number = post_number + 1,
			    date_last_post = NOW(),
			    last_post_author_user_id = ?
			WHERE id = ?`,
			authorUserID, req.TopicId)

		// Subforum last-post metadata
		if subforumID != 0 {
			var username string
			_ = db.QueryRow("SELECT username FROM users WHERE id = ?", authorUserID).Scan(&username)
			_, _ = db.Exec(`
				UPDATE subforums
				SET post_number              = COALESCE(post_number, 0) + 1,
				    last_post_topic_id       = ?,
				    last_post_topic_name     = ?,
				    last_post_id             = ?,
				    date_last_post           = NOW(),
				    last_post_author_user_name = ?,
				    show_last_topic          = false
				WHERE id = ?`,
				req.TopicId, topicName, postID, username, subforumID)
		}

		// User post counter
		if topicType == Entities.EpisodeTopic {
			_, _ = db.Exec("UPDATE users SET total_posts = total_posts + 1 WHERE id = ?", authorUserID)
		} else {
			_, _ = db.Exec("UPDATE users SET total_general_posts = total_general_posts + 1 WHERE id = ?", authorUserID)
		}

		// Character post counter (episode topics only)
		if topicType == Entities.EpisodeTopic && charID != 0 {
			_, _ = db.Exec(
				"UPDATE character_base SET total_posts = total_posts + 1, date_last_post = NOW() WHERE id = ?",
				charID)
		}

		publishedCount++
	}

	_, _ = db.Exec("UPDATE user_data_processing SET status = ? WHERE id = ?", UserDataProcessingStatusPublished, req.ProcessingId)

	c.JSON(http.StatusOK, gin.H{
		"published_posts": publishedCount,
		"skipped_posts":   skippedCount,
	})
}

package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"database/sql"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type CreateEpisodeRequest struct {
	SubforumID   int                    `json:"subforum_id" binding:"required"`
	Name         string                 `json:"name" binding:"required"`
	CharacterIDs []int                  `json:"character_ids"`
	CustomFields map[string]interface{} `json:"custom_fields"`
}

type GetEpisodesRequest struct {
	SubforumIDs  []int `json:"subforum_ids"`
	CharacterIDs []int `json:"character_ids"`
	FactionIDs   []int `json:"faction_ids"`
	Page         int   `json:"page"`
}

type EpisodeListItem struct {
	Id           int    `json:"id"`
	Name         string `json:"name"`
	TopicId      int    `json:"topic_id"`
	SubforumId   int    `json:"subforum_id"`
	SubforumName string `json:"subforum_name"`
	TopicStatus  int    `json:"topic_status"`
	LastPostDate string `json:"last_post_date"`
}

func CreateEpisode(c *gin.Context, db *sql.DB) {
	var req CreateEpisodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	userID := Services.GetUserIdFromContext(c)
	if userID == 0 {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusUnauthorized, Message: "Unauthorized"})
		c.Abort()
		return
	}

	tx, err := db.Begin()
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to start transaction"})
		c.Abort()
		return
	}
	defer tx.Rollback()

	// 1. Insert Topic (without first post)
	// Note: post_number = 0.
	res, err := tx.Exec("INSERT INTO topics (subforum_id, name, author_user_id, date_created, date_last_post, status, type, post_number, last_post_author_user_id) VALUES (?, ?, ?, NOW(), NOW(), 0, 1, 0, ?)",
		req.SubforumID, req.Name, userID, userID)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to insert topic: " + err.Error()})
		c.Abort()
		return
	}
	topicID, err := res.LastInsertId()
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get topic ID"})
		c.Abort()
		return
	}

	if err := tx.Commit(); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to commit transaction"})
		c.Abort()
		return
	}

	// 2. Create Episode Entity using Service
	episode := Entities.Episode{
		Topic_Id: int(topicID),
		Name:     req.Name,
		CustomFields: Entities.CustomFieldEntity{
			CustomFields: req.CustomFields,
		},
	}

	createdEntity, _, err := Services.CreateEntity("episode", &episode, db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to create episode entity: " + err.Error()})
		c.Abort()
		return
	}

	createdEpisode, ok := createdEntity.(*Entities.Episode)
	if !ok {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to cast created entity"})
		c.Abort()
		return
	}

	// 3. Insert Episode-Character Relations
	if len(req.CharacterIDs) > 0 {
		// Start a new transaction for relations
		txRel, err := db.Begin()
		if err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to start transaction for relations"})
			c.Abort()
			return
		}

		stmt, err := txRel.Prepare("INSERT INTO episode_character (episode_id, character_id) VALUES (?, ?)")
		if err != nil {
			txRel.Rollback()
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to prepare character relation statement"})
			c.Abort()
			return
		}
		defer stmt.Close()

		for _, charID := range req.CharacterIDs {
			_, err := stmt.Exec(createdEpisode.Id, charID)
			if err != nil {
				txRel.Rollback()
				_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to insert character relation: " + err.Error()})
				c.Abort()
				return
			}
		}

		if err := txRel.Commit(); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to commit relation transaction"})
			c.Abort()
			return
		}
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Episode created successfully", "episode_id": createdEpisode.Id, "topic_id": topicID})
}

func GetEpisodes(c *gin.Context, db *sql.DB) {
	var req GetEpisodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	query := `SELECT e.id, e.name, e.topic_id, t.subforum_id, s.name, t.status, t.date_last_post
		FROM episode_base e
		JOIN topics t ON e.topic_id = t.id
		JOIN subforums s ON t.subforum_id = s.id
		WHERE 1=1`
	var args []interface{}

	if len(req.SubforumIDs) > 0 {
		placeholders := make([]string, len(req.SubforumIDs))
		for i, id := range req.SubforumIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += " AND t.subforum_id IN (" + strings.Join(placeholders, ",") + ")"
	}

	if len(req.CharacterIDs) > 0 {
		placeholders := make([]string, len(req.CharacterIDs))
		for i, id := range req.CharacterIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += " AND EXISTS (SELECT 1 FROM episode_character ec WHERE ec.episode_id = e.id AND ec.character_id IN (" + strings.Join(placeholders, ",") + "))"
	}

	if len(req.FactionIDs) > 0 {
		placeholders := make([]string, len(req.FactionIDs))
		for i, id := range req.FactionIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += " AND EXISTS (SELECT 1 FROM episode_character ec JOIN character_faction cf ON ec.character_id = cf.character_id WHERE ec.episode_id = e.id AND cf.faction_id IN (" + strings.Join(placeholders, ",") + "))"
	}

	limit := 20
	page := req.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	query += " ORDER BY t.date_last_post DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get episodes: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	var episodes []EpisodeListItem = []EpisodeListItem{}
	for rows.Next() {
		var ep EpisodeListItem
		if err := rows.Scan(&ep.Id, &ep.Name, &ep.TopicId, &ep.SubforumId, &ep.SubforumName, &ep.TopicStatus, &ep.LastPostDate); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan episode: " + err.Error()})
			c.Abort()
			return
		}
		episodes = append(episodes, ep)
	}

	c.JSON(http.StatusOK, episodes)
}

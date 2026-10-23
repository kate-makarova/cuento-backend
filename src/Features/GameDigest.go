package Features

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type GameDigestFeature struct{}

func (GameDigestFeature) Name() string        { return "AI Game Digest" }
func (GameDigestFeature) Description() string { return "AI-generated digest of game events" }
func (GameDigestFeature) Key() string         { return "game_digest" }
func (GameDigestFeature) IsActive() bool      { return false }

type GameDigest struct {
	ID               int       `json:"id"`
	ImplementationID int       `json:"implementation_id"`
	Datetime         time.Time `json:"datetime"`
	Text             string    `json:"text"`
	IsPublished      bool      `json:"is_published"`
	PublishedPostID  *int      `json:"published_post_id"`
	PublishedUserID  *int      `json:"published_user_id"`
}

type GameDigestEpisodeNote struct {
	EpisodeID int64            `json:"episode_id"`
	Notes     *json.RawMessage `json:"notes"`
}

type GameDigestSetting struct {
	SettingName  string           `json:"setting_name"`
	SettingValue *json.RawMessage `json:"setting_value"`
}

func GetGameDigestsHandler(c *gin.Context, db *sql.DB) {
	implIDStr := c.DefaultQuery("implementation_id", "")
	query := `SELECT id, implementation_id, datetime, text, is_published, published_post_id, published_user_id FROM game_digests`
	args := []interface{}{}
	if implIDStr != "" {
		if id, err := strconv.Atoi(implIDStr); err == nil {
			query += " WHERE implementation_id = ?"
			args = append(args, id)
		}
	}
	query += " ORDER BY datetime DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch digests"})
		return
	}
	defer rows.Close()

	items := []GameDigest{}
	for rows.Next() {
		var d GameDigest
		if err := rows.Scan(&d.ID, &d.ImplementationID, &d.Datetime, &d.Text, &d.IsPublished, &d.PublishedPostID, &d.PublishedUserID); err != nil {
			continue
		}
		items = append(items, d)
	}
	c.JSON(http.StatusOK, items)
}

func GetGameDigestHandler(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
		return
	}
	var d GameDigest
	err = db.QueryRow(
		`SELECT id, implementation_id, datetime, text, is_published, published_post_id, published_user_id FROM game_digests WHERE id = ?`, id,
	).Scan(&d.ID, &d.ImplementationID, &d.Datetime, &d.Text, &d.IsPublished, &d.PublishedPostID, &d.PublishedUserID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Digest not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch digest"})
		return
	}
	c.JSON(http.StatusOK, d)
}

func UpdateGameDigestHandler(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
		return
	}
	var req struct {
		IsPublished     *bool `json:"is_published"`
		PublishedPostID *int  `json:"published_post_id"`
		PublishedUserID *int  `json:"published_user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}
	if req.IsPublished != nil {
		if _, err := db.Exec("UPDATE game_digests SET is_published = ? WHERE id = ?", *req.IsPublished, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update digest"})
			return
		}
	}
	if req.PublishedPostID != nil {
		if _, err := db.Exec("UPDATE game_digests SET published_post_id = ? WHERE id = ?", *req.PublishedPostID, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update digest"})
			return
		}
	}
	if req.PublishedUserID != nil {
		if _, err := db.Exec("UPDATE game_digests SET published_user_id = ? WHERE id = ?", *req.PublishedUserID, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update digest"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "Digest updated"})
}

func GetEpisodeNotesHandler(c *gin.Context, db *sql.DB) {
	episodeID, err := strconv.ParseInt(c.Param("episode_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid episode_id"})
		return
	}
	var note GameDigestEpisodeNote
	note.EpisodeID = episodeID
	err = db.QueryRow(`SELECT notes FROM game_digest_episode_notes WHERE episode_id = ?`, episodeID).Scan(&note.Notes)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusOK, note)
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notes"})
		return
	}
	c.JSON(http.StatusOK, note)
}

func SaveEpisodeNotesHandler(c *gin.Context, db *sql.DB) {
	episodeID, err := strconv.ParseInt(c.Param("episode_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid episode_id"})
		return
	}
	var req struct {
		Notes *json.RawMessage `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}
	_, err = db.Exec(
		`INSERT INTO game_digest_episode_notes (episode_id, notes) VALUES (?, ?) ON DUPLICATE KEY UPDATE notes = VALUES(notes)`,
		episodeID, req.Notes,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save notes"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Notes saved"})
}

func GetGameDigestSettingsHandler(c *gin.Context, db *sql.DB) {
	rows, err := db.Query(`SELECT setting_name, setting_value FROM game_digest_settings`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch settings"})
		return
	}
	defer rows.Close()

	items := []GameDigestSetting{}
	for rows.Next() {
		var s GameDigestSetting
		if err := rows.Scan(&s.SettingName, &s.SettingValue); err != nil {
			continue
		}
		items = append(items, s)
	}
	c.JSON(http.StatusOK, items)
}

func SaveGameDigestSettingHandler(c *gin.Context, db *sql.DB) {
	var req GameDigestSetting
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}
	if req.SettingName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "setting_name is required"})
		return
	}
	_, err := db.Exec(
		`INSERT INTO game_digest_settings (setting_name, setting_value) VALUES (?, ?) ON DUPLICATE KEY UPDATE setting_value = VALUES(setting_value)`,
		req.SettingName, req.SettingValue,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save setting"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Setting saved"})
}

package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type OverallStatsResponse struct {
	CreatedEpisodes         int `json:"created_episodes"`
	FinishedEpisodes        int `json:"finished_episodes"`
	InactivatedEpisodes     int `json:"inactivated_episodes"`
	TotalPosts              int `json:"total_posts"`
	CreatedWantedCharacters int `json:"created_wanted_characters"`
	AcceptedCharacters      int `json:"accepted_characters"`
}

func GetOverallStats(c *gin.Context, db *sql.DB) {
	dateFromStr := c.Query("date_from")
	dateToStr := c.Query("date_to")

	dateFrom, err := time.Parse("2006-01-02", dateFromStr)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid date_from: expected YYYY-MM-DD"})
		c.Abort()
		return
	}
	dateTo, err := time.Parse("2006-01-02", dateToStr)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid date_to: expected YYYY-MM-DD"})
		c.Abort()
		return
	}
	// Make date_to inclusive of the full day.
	dateTo = dateTo.Add(24*time.Hour - time.Second)

	var stats OverallStatsResponse

	_ = db.QueryRow(`
		SELECT COUNT(*) FROM episode_base eb
		JOIN topics t ON t.id = eb.topic_id
		WHERE t.date_created BETWEEN ? AND ?
	`, dateFrom, dateTo).Scan(&stats.CreatedEpisodes)

	_ = db.QueryRow(`
		SELECT COUNT(DISTINCT topic_id) FROM topic_activity_log
		WHERE event = 'episode_status_changed'
		  AND new_state = ?
		  AND date BETWEEN ? AND ?
	`, int(Entities.FinishedEpisode), dateFrom, dateTo).Scan(&stats.FinishedEpisodes)

	_ = db.QueryRow(`
		SELECT COUNT(DISTINCT topic_id) FROM topic_activity_log
		WHERE event = 'episode_status_changed'
		  AND new_state = ?
		  AND date BETWEEN ? AND ?
	`, int(Entities.InactiveEpisode), dateFrom, dateTo).Scan(&stats.InactivatedEpisodes)

	_ = db.QueryRow(`
		SELECT COUNT(*) FROM posts
		WHERE date_created BETWEEN ? AND ?
		  AND (is_deleted IS NULL OR is_deleted != 1)
	`, dateFrom, dateTo).Scan(&stats.TotalPosts)

	_ = db.QueryRow(`
		SELECT COUNT(*) FROM wanted_character_base wb
		JOIN topics t ON t.id = wb.topic_id
		WHERE t.date_created BETWEEN ? AND ?
	`, dateFrom, dateTo).Scan(&stats.CreatedWantedCharacters)

	_ = db.QueryRow(`
		SELECT COUNT(DISTINCT topic_id) FROM topic_activity_log
		WHERE event = 'character_status_changed'
		  AND new_state = ?
		  AND date BETWEEN ? AND ?
	`, int(Entities.ActiveCharacter), dateFrom, dateTo).Scan(&stats.AcceptedCharacters)

	c.JSON(http.StatusOK, stats)
}

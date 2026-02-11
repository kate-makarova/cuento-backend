package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

type BoardInfo struct {
	SiteName               string              `json:"site_name"`
	Domain                 string              `json:"domain"`
	TotalUserNumber        int                 `json:"total_user_number"`
	TotalCharacterNumber   int                 `json:"total_character_number"`
	TotalEpisodeNumber     int                 `json:"total_episode_number"`
	TotalTopicNumber       int                 `json:"total_topic_number"`
	TotalPostNumber        int                 `json:"total_post_number"`
	TotalEpisodePostNumber int                 `json:"total_episode_post_number"`
	LastRegisteredUser     *Entities.ShortUser `json:"last_registered_user"`
}

func GetBoard(c *gin.Context, db *sql.DB) {
	var boardInfo = BoardInfo{
		TotalUserNumber:        0,
		TotalCharacterNumber:   0,
		TotalEpisodeNumber:     0,
		TotalTopicNumber:       0,
		TotalPostNumber:        0,
		TotalEpisodePostNumber: 0,
		LastRegisteredUser:     nil,
	}

	rows, err := db.Query("SELECT setting_name, setting_value FROM global_settings WHERE setting_name IN ('site_name', 'domain')")
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get global settings: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan settings: " + err.Error()})
			c.Abort()
			return
		}
		switch name {
		case "site_name":
			boardInfo.SiteName = value
		case "domain":
			boardInfo.Domain = value
		}
	}

	rows, err = db.Query("SELECT stat_name, stat_value FROM global_stats WHERE stat_name IN ('total_user_number', 'total_character_number', 'total_episode_number', 'total_topic_number', 'total_post_number', 'total_episode_post_number')")
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get global stats: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var value int
		if err := rows.Scan(&name, &value); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan stats: " + err.Error()})
			c.Abort()
			return
		}
		switch name {
		case "total_user_number":
			boardInfo.TotalUserNumber = value
		case "total_character_number":
			boardInfo.TotalCharacterNumber = value
		case "total_episode_number":
			boardInfo.TotalEpisodeNumber = value
		case "total_topic_number":
			boardInfo.TotalTopicNumber = value
		case "total_post_number":
			boardInfo.TotalPostNumber = value
		case "total_episode_post_number":
			boardInfo.TotalEpisodePostNumber = value
		}
	}

	c.JSON(http.StatusOK, boardInfo)
}

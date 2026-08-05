package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/MCP"
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

// superuserOnlySettings is the hardcoded set of settings only the superuser may
// read or write. Keeping this in code (not the DB) means a compromised database
// cannot expose or unlock these settings for other users.
var superuserOnlySettings = map[string]bool{
	// Site domain
	"domain": true,
	// Image uploading
	"use_image_uploading": true,
	"imgbb_api_key":       true,
	// AI
	"ai_api_key": true,
	"ai_name":    true,
	"ai_model":   true,
	// GitHub
	"github_token":  true,
	"github_owner":  true,
	"github_repo":   true,
	"github_branch": true,
}

func GetGlobalSettings(c *gin.Context, db *sql.DB) {
	rows, err := db.Query("SELECT setting_name, setting_value FROM global_settings")
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get settings: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	isSuperuser := Services.IsSuperuser(c)
	var settings []Entities.Setting
	for rows.Next() {
		var s Entities.Setting
		var value sql.NullString
		if err := rows.Scan(&s.SettingName, &value); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan setting: " + err.Error()})
			c.Abort()
			return
		}
		if superuserOnlySettings[s.SettingName] && !isSuperuser {
			continue
		}
		s.SettingValue = value.String
		settings = append(settings, s)
	}

	if settings == nil {
		settings = []Entities.Setting{}
	}

	c.JSON(http.StatusOK, settings)
}

func UpdateGlobalSettings(c *gin.Context, db *sql.DB) {
	var req []Entities.Setting
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	isSuperuser := Services.IsSuperuser(c)
	for _, s := range req {
		if superuserOnlySettings[s.SettingName] && !isSuperuser {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusForbidden, Message: "You do not have permission to change setting: " + s.SettingName})
			c.Abort()
			return
		}
	}

	tx, err := db.Begin()
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to start transaction"})
		c.Abort()
		return
	}
	defer tx.Rollback()

	for _, s := range req {
		_, err := tx.Exec("UPDATE global_settings SET setting_value = ? WHERE setting_name = ?", s.SettingValue, s.SettingName)
		if err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to update setting: " + err.Error()})
			c.Abort()
			return
		}
	}

	if err := tx.Commit(); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to commit transaction"})
		c.Abort()
		return
	}

	aiSettings := map[string]bool{"ai_api_key": true, "ai_name": true, "ai_model": true}
	for _, s := range req {
		if aiSettings[s.SettingName] {
			MCP.ReinitializeAgent(db)
			break
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Settings updated"})
}

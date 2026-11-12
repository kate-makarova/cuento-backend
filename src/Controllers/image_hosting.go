package Controllers

import (
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type imageHostingSettingsResponse struct {
	Enabled        string `json:"enabled"`
	Storage        string `json:"storage"`
	RamLimitMB     string `json:"ram_limit_mb"`
	S3Endpoint     string `json:"s3_endpoint"`
	S3Bucket       string `json:"s3_bucket"`
	S3AccessKey    string `json:"s3_access_key"`
	S3SecretKey    string `json:"s3_secret_key"`
	NginxActive    bool   `json:"nginx_active"`
	UploadProvider string `json:"upload_provider"`
	BaseURL        string `json:"base_url"`
	ImgbbApiKey    string `json:"imgbb_api_key"`
}

type imageHostingSettingsRequest struct {
	Enabled        string `json:"enabled"`
	Storage        string `json:"storage"`
	RamLimitMB     string `json:"ram_limit_mb"`
	S3Endpoint     string `json:"s3_endpoint"`
	S3Bucket       string `json:"s3_bucket"`
	S3AccessKey    string `json:"s3_access_key"`
	S3SecretKey    string `json:"s3_secret_key"`
	UploadProvider string `json:"upload_provider"`
	BaseURL        string `json:"base_url"`
	ImgbbApiKey    string `json:"imgbb_api_key"`
}

var imageHostingSettingKeys = []string{
	"image_hosting_enabled",
	"image_hosting_storage",
	"image_hosting_ram_limit_mb",
	"image_hosting_s3_endpoint",
	"image_hosting_s3_bucket",
	"image_hosting_s3_access_key",
	"image_hosting_s3_secret_key",
}

func GetImageHostingSettings(c *gin.Context, db *sql.DB) {
	rows, err := db.Query(
		"SELECT setting_name, setting_value FROM global_settings WHERE setting_name IN (?,?,?,?,?,?,?,?,?,?)",
		"image_hosting_enabled", "image_hosting_storage", "image_hosting_ram_limit_mb",
		"image_hosting_s3_endpoint", "image_hosting_s3_bucket",
		"image_hosting_s3_access_key", "image_hosting_s3_secret_key",
		"image_upload_provider", "image_hosting_base_url", "imgbb_api_key",
	)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to load image hosting settings: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	settings := map[string]string{}
	for rows.Next() {
		var name string
		var value sql.NullString
		if err := rows.Scan(&name, &value); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan setting: " + err.Error()})
			c.Abort()
			return
		}
		settings[name] = value.String
	}

	isSuperuser := Services.IsSuperuser(c)
	resp := imageHostingSettingsResponse{
		Enabled:        settings["image_hosting_enabled"],
		Storage:        settings["image_hosting_storage"],
		RamLimitMB:     settings["image_hosting_ram_limit_mb"],
		S3Endpoint:     settings["image_hosting_s3_endpoint"],
		S3Bucket:       settings["image_hosting_s3_bucket"],
		NginxActive:    Services.IsNginxImageHostingActive(),
		UploadProvider: settings["image_upload_provider"],
		BaseURL:        settings["image_hosting_base_url"],
		ImgbbApiKey:    settings["imgbb_api_key"],
	}
	if isSuperuser {
		resp.S3AccessKey = settings["image_hosting_s3_access_key"]
		resp.S3SecretKey = settings["image_hosting_s3_secret_key"]
	}

	c.JSON(http.StatusOK, resp)
}

func UpdateImageHostingSettings(c *gin.Context, db *sql.DB) {
	var req imageHostingSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	if req.Storage != "local" && req.Storage != "s3" {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "storage must be 'local' or 's3'"})
		c.Abort()
		return
	}
	if req.UploadProvider != "imgbb" && req.UploadProvider != "local" {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "upload_provider must be 'imgbb' or 'local'"})
		c.Abort()
		return
	}
	if req.Enabled != "y" && req.Enabled != "n" {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "enabled must be 'y' or 'n'"})
		c.Abort()
		return
	}
	ramMB, err := strconv.ParseInt(req.RamLimitMB, 10, 64)
	if err != nil || ramMB <= 0 {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "ram_limit_mb must be a positive integer"})
		c.Abort()
		return
	}

	isSuperuser := Services.IsSuperuser(c)
	if !isSuperuser && (req.S3AccessKey != "" || req.S3SecretKey != "") {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusForbidden, Message: "Only the superuser can change S3 credentials"})
		c.Abort()
		return
	}

	currentEnabled, _ := Services.GetGlobalSetting("image_hosting_enabled", db)

	tx, err := db.Begin()
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to start transaction"})
		c.Abort()
		return
	}
	defer tx.Rollback()

	updates := map[string]string{
		"image_hosting_enabled":      req.Enabled,
		"image_hosting_storage":      req.Storage,
		"image_hosting_ram_limit_mb": req.RamLimitMB,
		"image_hosting_s3_endpoint":  req.S3Endpoint,
		"image_hosting_s3_bucket":    req.S3Bucket,
		"image_upload_provider":      req.UploadProvider,
		"image_hosting_base_url":     req.BaseURL,
		"imgbb_api_key":              req.ImgbbApiKey,
	}
	if isSuperuser {
		updates["image_hosting_s3_access_key"] = req.S3AccessKey
		updates["image_hosting_s3_secret_key"] = req.S3SecretKey
	}

	for name, value := range updates {
		if _, err := tx.Exec("UPDATE global_settings SET setting_value = ? WHERE setting_name = ?", value, name); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to update setting: " + err.Error()})
			c.Abort()
			return
		}
	}

	if err := tx.Commit(); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to commit settings"})
		c.Abort()
		return
	}

	message := "Settings updated"
	if req.Enabled != currentEnabled {
		var nginxErr error
		if req.Enabled == "y" {
			nginxErr = Services.EnableImageHosting()
		} else {
			nginxErr = Services.DisableImageHosting()
		}
		if nginxErr != nil {
			message = "Settings saved. Could not apply nginx change: " + nginxErr.Error()
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": message})
}

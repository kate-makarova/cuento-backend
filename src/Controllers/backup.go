package Controllers

import (
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"database/sql"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func BackupDB(c *gin.Context) {
	if !Services.IsSuperuser(c) {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusForbidden, Message: "Forbidden"})
		c.Abort()
		return
	}

	filename := "backup_" + time.Now().UTC().Format("2006-01-02_15-04-05") + ".sql"
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", "application/sql")
	c.Status(http.StatusOK)

	if err := Services.MysqldumpToWriter(c.Writer); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Backup failed: " + err.Error()})
		c.Abort()
	}
}

func RestoreDB(c *gin.Context) {
	if !Services.IsSuperuser(c) {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusForbidden, Message: "Forbidden"})
		c.Abort()
		return
	}

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "SQL file is required"})
		c.Abort()
		return
	}
	defer file.Close()

	if err := Services.MysqlRestoreFromReader(file); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Restore failed: " + err.Error()})
		c.Abort()
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Database restored successfully"})
}

type GDriveBackupSettings struct {
	ServiceAccountKey string               `json:"service_account_key"`
	Enabled           bool                 `json:"enabled"`
	Frequency         string               `json:"frequency"`
	Keep              int                  `json:"keep"`
	LastRun           *time.Time           `json:"last_run"`
	Backups           []Services.BackupFile `json:"backups"`
}

func GetGDriveBackupSettings(c *gin.Context, db *sql.DB) {
	if !Services.IsSuperuser(c) {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusForbidden, Message: "Forbidden"})
		c.Abort()
		return
	}

	key, _ := Services.GetGlobalSetting("gdrive_service_account_key", db)
	enabled, _ := Services.GetGlobalSetting("gdrive_backup_enabled", db)
	frequency, _ := Services.GetGlobalSetting("gdrive_backup_frequency", db)
	keepStr, _ := Services.GetGlobalSetting("gdrive_backup_keep", db)
	lastRunStr, _ := Services.GetGlobalSetting("gdrive_backup_last_run", db)

	if frequency == "" {
		frequency = "daily"
	}
	keep := 5
	if n, err := strconv.Atoi(keepStr); err == nil && n > 0 {
		keep = n
	}

	var lastRun *time.Time
	if lastRunStr != "" {
		if t, err := time.Parse(time.RFC3339, lastRunStr); err == nil {
			lastRun = &t
		}
	}

	var backups []Services.BackupFile
	if key != "" {
		backups, _ = Services.NewGDriveProvider(db).ListBackups()
	}
	if backups == nil {
		backups = []Services.BackupFile{}
	}

	c.JSON(http.StatusOK, GDriveBackupSettings{
		ServiceAccountKey: key,
		Enabled:           enabled == "y",
		Frequency:         frequency,
		Keep:              keep,
		LastRun:           lastRun,
		Backups:           backups,
	})
}

type UpdateGDriveBackupSettingsRequest struct {
	ServiceAccountKey string `json:"service_account_key"`
	Enabled           bool   `json:"enabled"`
	Frequency         string `json:"frequency"`
	Keep              int    `json:"keep"`
}

func UpdateGDriveBackupSettings(c *gin.Context, db *sql.DB) {
	if !Services.IsSuperuser(c) {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusForbidden, Message: "Forbidden"})
		c.Abort()
		return
	}

	var req UpdateGDriveBackupSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request"})
		c.Abort()
		return
	}

	if req.Frequency != "daily" && req.Frequency != "weekly" && req.Frequency != "monthly" {
		req.Frequency = "daily"
	}
	if req.Keep < 1 {
		req.Keep = 1
	}

	enabledVal := "n"
	if req.Enabled {
		enabledVal = "y"
	}

	upsert := func(name, value string) error {
		_, err := db.Exec(`INSERT INTO global_settings (setting_name, setting_value, needs_superuser)
			VALUES (?, ?, 1) ON DUPLICATE KEY UPDATE setting_value = VALUES(setting_value)`, name, value)
		return err
	}

	if err := upsert("gdrive_service_account_key", req.ServiceAccountKey); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to save settings"})
		c.Abort()
		return
	}
	upsert("gdrive_backup_enabled", enabledVal)
	upsert("gdrive_backup_frequency", req.Frequency)
	upsert("gdrive_backup_keep", strconv.Itoa(req.Keep))

	c.JSON(http.StatusOK, gin.H{"message": "Settings saved"})
}

func DownloadGDriveBackup(c *gin.Context, db *sql.DB) {
	if !Services.IsSuperuser(c) {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusForbidden, Message: "Forbidden"})
		c.Abort()
		return
	}

	fileID := c.Param("fileId")
	if fileID == "" {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Missing file ID"})
		c.Abort()
		return
	}

	provider := Services.NewGDriveProvider(db)
	reader, filename, err := provider.Download(fileID)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Download failed: " + err.Error()})
		c.Abort()
		return
	}
	defer reader.Close()

	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", "application/sql")
	c.Status(http.StatusOK)
	io.Copy(c.Writer, reader)
}

func TriggerGDriveBackup(c *gin.Context, db *sql.DB) {
	if !Services.IsSuperuser(c) {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusForbidden, Message: "Forbidden"})
		c.Abort()
		return
	}
	if err := Services.RunCloudBackup(db); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Backup failed: " + err.Error()})
		c.Abort()
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Backup completed"})
}

package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func GetFactionChildren(c *gin.Context, db *sql.DB) {
	parentIDStr := c.Param("parent_id")

	var rows *sql.Rows
	var err error

	if parentIDStr == "" || parentIDStr == "0" {
		rows, err = db.Query("SELECT id, name, parent_id, level, description, icon, show_on_profile FROM factions WHERE parent_id IS NULL ORDER BY name")
	} else {
		parentID, convErr := strconv.Atoi(parentIDStr)
		if convErr != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid parent_id"})
			c.Abort()
			return
		}
		rows, err = db.Query("SELECT id, name, parent_id, level, description, icon, show_on_profile FROM factions WHERE parent_id = ? ORDER BY name", parentID)
	}

	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get factions: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	var factions []Entities.Faction
	for rows.Next() {
		var f Entities.Faction
		if err := rows.Scan(&f.Id, &f.Name, &f.ParentId, &f.Level, &f.Description, &f.Icon, &f.ShowOnProfile); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan faction: " + err.Error()})
			c.Abort()
			return
		}
		factions = append(factions, f)
	}

	if factions == nil {
		factions = []Entities.Faction{}
	}

	c.JSON(http.StatusOK, factions)
}

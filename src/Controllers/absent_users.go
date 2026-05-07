package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type AbsentUserItem struct {
	UserId           int                       `json:"user_id"`
	Username         string                    `json:"username"`
	AbsenceStartDate time.Time                 `json:"absence_start_date"`
	AbsenceEndDate   time.Time                 `json:"absence_end_date"`
	Characters       []Entities.ShortCharacter `json:"characters"`
}

func GetAbsentUsers(c *gin.Context, db *sql.DB) {
	rows, err := db.Query(`
		SELECT au.user_id, u.username, au.absence_start_date, au.absence_end_date
		FROM absent_users au
		JOIN users u ON u.id = au.user_id
		WHERE au.absence_start_date <= NOW() AND au.absence_end_date >= NOW()
		ORDER BY au.absence_end_date ASC
	`)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get absent users: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	users := []AbsentUserItem{}
	for rows.Next() {
		var u AbsentUserItem
		if err := rows.Scan(&u.UserId, &u.Username, &u.AbsenceStartDate, &u.AbsenceEndDate); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan absent user: " + err.Error()})
			c.Abort()
			return
		}

		u.Characters = []Entities.ShortCharacter{}
		charRows, err := db.Query(
			"SELECT id, name FROM character_base WHERE user_id = ? AND character_status = ? ORDER BY name ASC",
			u.UserId, Entities.ActiveCharacter,
		)
		if err == nil {
			for charRows.Next() {
				var ch Entities.ShortCharacter
				if err := charRows.Scan(&ch.Id, &ch.Name); err == nil {
					u.Characters = append(u.Characters, ch)
				}
			}
			charRows.Close()
		}

		users = append(users, u)
	}

	c.JSON(http.StatusOK, users)
}

package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetHomeCategories(c *gin.Context, db *sql.DB) {
	rows, err := db.Query("SELECT" +
		"   subforums.id as subforum_id," +
		"    subforums.name as subforum_name," +
		"    subforums.description," +
		"    subforums.position as subforum_position," +
		"   subforums.topic_number," +
		"    subforums.post_number," +
		"    categories.id as category_id," +
		"    categories.name as category_name," +
		"    categories.position as category_position" +
		"    FROM subforums" +
		"    JOIN categories on subforums.category_id = categories.id" +
		"    ORDER BY category_position, subforum_position")
	if err != nil && err != sql.ErrNoRows {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get categories: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	var categories []Entities.Category
	var category Entities.Category
	for rows.Next() {
		var tempSubforum Entities.Subform
		var tempCategory Entities.Category
		if err := rows.Scan(
			&tempSubforum.Id,
			&tempSubforum.Name,
			&tempSubforum.Description,
			&tempSubforum.Position,
			&tempSubforum.TopicNumber,
			&tempSubforum.PostNumber,
			&tempCategory.Id,
			&tempCategory.Name,
			&tempCategory.Position,
		); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan categories: " + err.Error()})
		}
		if category.Id != tempCategory.Id {
			if category.Id != 0 {
				categories = append(categories, category)
			}
			category = tempCategory
		}
		category.Subforums = append(category.Subforums, tempSubforum)
	}
	categories = append(categories, category)

	c.JSON(http.StatusOK, categories)
}

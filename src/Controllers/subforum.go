package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func GetHomeCategories(c *gin.Context, db *sql.DB) {
	// 1. Fetch all subforums and categories
	rows, err := db.Query("SELECT" +
		"   subforums.id as subforum_id," +
		"    subforums.name as subforum_name," +
		"    subforums.description," +
		"    subforums.position as subforum_position," +
		"   subforums.topic_number," +
		"    subforums.post_number," +
		"    subforums.last_post_topic_id," +
		"    subforums.last_post_topic_name," +
		"    subforums.last_post_id," +
		"    subforums.date_last_post, " +
		"    subforums.last_post_author_user_name," +
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

	// Store raw rows temporarily to filter them later
	type categoryRow struct {
		Subforum Entities.Subform
		Category Entities.Category
	}
	var allRows []categoryRow

	for rows.Next() {
		var row categoryRow
		if err := rows.Scan(
			&row.Subforum.Id,
			&row.Subforum.Name,
			&row.Subforum.Description,
			&row.Subforum.Position,
			&row.Subforum.TopicNumber,
			&row.Subforum.PostNumber,
			&row.Subforum.LastPostTopicId,
			&row.Subforum.LastPostTopicName,
			&row.Subforum.LastPostId,
			&row.Subforum.DateLastPost,
			&row.Subforum.LastPostAuthorName,
			&row.Category.Id,
			&row.Category.Name,
			&row.Category.Position,
		); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan categories: " + err.Error()})
			c.Abort()
			return
		}
		allRows = append(allRows, row)
	}

	// 2. Determine User Roles
	var roleIDs []int
	userID := Services.GetUserIdFromContext(c)
	if userID > 0 {
		// Fetch roles for logged-in user
		rows, err := db.Query("SELECT role_id FROM user_role WHERE user_id = ?", userID)
		if err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get user roles: " + err.Error()})
			c.Abort()
			return
		}
		defer rows.Close()
		for rows.Next() {
			var rID int
			if err := rows.Scan(&rID); err == nil {
				roleIDs = append(roleIDs, rID)
			}
		}
	}

	if len(roleIDs) == 0 {
		// Fallback to Guest role if no user or no roles found
		var guestID int
		err := db.QueryRow("SELECT id FROM roles WHERE name = 'Guest'").Scan(&guestID)
		if err == nil {
			roleIDs = append(roleIDs, guestID)
		}
	}

	// If we still have no roles, return an empty list (default deny)
	if len(roleIDs) == 0 {
		c.JSON(http.StatusOK, []Entities.Category{})
		return
	}

	// 3. Check Permissions
	// Build list of required permissions: "subforum_read:<id>"
	var permissionStrings []string
	var permissionArgs []interface{}
	seenPerms := make(map[string]bool)

	for _, r := range allRows {
		perm := fmt.Sprintf("subforum_read:%d", r.Subforum.Id)
		if !seenPerms[perm] {
			permissionStrings = append(permissionStrings, perm)
			permissionArgs = append(permissionArgs, perm)
			seenPerms[perm] = true
		}
	}

	allowedPerms := make(map[string]bool)
	if len(permissionStrings) > 0 {
		// Helper to build IN clause placeholders
		placeholders := func(n int) string {
			if n <= 0 {
				return ""
			}
			return strings.Repeat("?,", n-1) + "?"
		}

		query := fmt.Sprintf("SELECT permission FROM role_permission WHERE type = 1 AND role_id IN (%s) AND permission IN (%s)",
			placeholders(len(roleIDs)),
			placeholders(len(permissionStrings)))

		args := []interface{}{}
		for _, id := range roleIDs {
			args = append(args, id)
		}
		args = append(args, permissionArgs...)

		permRows, err := db.Query(query, args...)
		if err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to check permissions: " + err.Error()})
			c.Abort()
			return
		}
		defer permRows.Close()

		for permRows.Next() {
			var p string
			permRows.Scan(&p)
			allowedPerms[p] = true
		}
	}

	// 4. Filter and Group Results
	categories := []Entities.Category{}

	for _, r := range allRows {
		perm := fmt.Sprintf("subforum_read:%d", r.Subforum.Id)
		if allowedPerms[perm] {
			// Check if we need to start a new category block
			if len(categories) == 0 || categories[len(categories)-1].Id != r.Category.Id {
				cat := r.Category
				cat.Subforums = []Entities.Subform{}
				categories = append(categories, cat)
			}
			// Append subforum to the current (last added) category
			categories[len(categories)-1].Subforums = append(categories[len(categories)-1].Subforums, r.Subforum)
		}
	}

	c.JSON(http.StatusOK, categories)
}

func GetShortSubforumList(c *gin.Context, db *sql.DB) {
	rows, err := db.Query("SELECT id, name FROM subforums ORDER BY position")
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get subforums: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()
	var subforums []Entities.ShortSubform
	for rows.Next() {
		var tempSubforum Entities.ShortSubform
		if err := rows.Scan(&tempSubforum.Id, &tempSubforum.Name); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan subforums: " + err.Error()})
		}
		subforums = append(subforums, tempSubforum)
	}
	c.JSON(http.StatusOK, subforums)
}

func GetSubforum(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid ID"})
		c.Abort()
		return
	}

	var subforum Entities.Subform
	query := "SELECT id, category_id, name, description, position, topic_number, post_number, last_post_topic_id, last_post_topic_name, last_post_id, date_last_post, last_post_author_user_name FROM subforums WHERE id = ?"
	err = db.QueryRow(query, id).Scan(
		&subforum.Id,
		&subforum.CategoryId,
		&subforum.Name,
		&subforum.Description,
		&subforum.Position,
		&subforum.TopicNumber,
		&subforum.PostNumber,
		&subforum.LastPostTopicId,
		&subforum.LastPostTopicName,
		&subforum.LastPostId,
		&subforum.DateLastPost,
		&subforum.LastPostAuthorName,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "Subforum not found"})
		} else {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get subforum: " + err.Error()})
		}
		c.Abort()
		return
	}

	// Determine User Roles
	var roleIDs []int
	userID := Services.GetUserIdFromContext(c)
	if userID > 0 {
		rows, err := db.Query("SELECT role_id FROM user_role WHERE user_id = ?", userID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var rID int
				if err := rows.Scan(&rID); err == nil {
					roleIDs = append(roleIDs, rID)
				}
			}
		}
	}

	if len(roleIDs) == 0 {
		var guestID int
		err := db.QueryRow("SELECT id FROM roles WHERE name = 'Guest'").Scan(&guestID)
		if err == nil {
			roleIDs = append(roleIDs, guestID)
		}
	}

	// Check Permissions
	permissions := &Entities.SubforumPermissions{}
	subforum.Permissions = permissions

	if len(roleIDs) > 0 {
		permMap := map[string]*bool{
			fmt.Sprintf("subforum_create_general_topic:%d", id):   &permissions.SubforumCreateGeneralTopic,
			fmt.Sprintf("subforum_create_episode_topic:%d", id):   &permissions.SubforumCreateEpisodeTopic,
			fmt.Sprintf("subforum_create_character_topic:%d", id): &permissions.SubforumCreateCharacterTopic,
			fmt.Sprintf("subforum_post:%d", id):                   &permissions.SubforumPost,
			fmt.Sprintf("subforum_delete_topic:%d", id):           &permissions.SubforumDeleteOwnTopic,
			fmt.Sprintf("subforum_delete_others_topic:%d", id):    &permissions.SubforumDeleteOthersTopic,
			fmt.Sprintf("subforum_edit_others_post:%d", id):       &permissions.SubforumEditOthersPost,
			fmt.Sprintf("subforum_edit_own_post:%d", id):          &permissions.SubforumEditOwnPost,
		}

		var permStrings []string
		var args []interface{}
		for p := range permMap {
			permStrings = append(permStrings, p)
		}

		placeholders := func(n int) string {
			if n <= 0 {
				return ""
			}
			return strings.Repeat("?,", n-1) + "?"
		}

		query := fmt.Sprintf("SELECT permission FROM role_permission WHERE type = 1 AND role_id IN (%s) AND permission IN (%s)",
			placeholders(len(roleIDs)),
			placeholders(len(permStrings)))

		for _, rID := range roleIDs {
			args = append(args, rID)
		}
		for _, p := range permStrings {
			args = append(args, p)
		}

		rows, err := db.Query(query, args...)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var p string
				if err := rows.Scan(&p); err == nil {
					if val, ok := permMap[p]; ok {
						*val = true
					}
				}
			}
		}
	}

	c.JSON(http.StatusOK, subforum)
}

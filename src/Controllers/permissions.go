package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Router"
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

type PermissionType int

const (
	EndpointPermission PermissionType = 0
	SubforumPermission PermissionType = 1
)

var SubforumPermissions = []string{
	"subforum_read",
	"subforum_create_general_topic",
	"subforum_create_episode_topic",
	"subforum_create_character_topic",
	"subforum_post",
	"subforum_delete_topic",
	"subforum_delete_others_topic",
	"subforum_edit_others_post",
	"subforum_edit_own_post",
}

func GetPermissionMatrix(c *gin.Context, db *sql.DB) {
	// 1. Fetch all roles
	roleRows, err := db.Query("SELECT id, name FROM roles")
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to fetch roles: " + err.Error()})
		c.Abort()
		return
	}
	defer roleRows.Close()

	roles := make([]Entities.Role, 0)
	roleMap := make(map[int]string)
	for roleRows.Next() {
		var role Entities.Role
		if err := roleRows.Scan(&role.Id, &role.Name); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan role: " + err.Error()})
			c.Abort()
			return
		}
		roles = append(roles, role)
		roleMap[role.Id] = role.Name
	}

	// 2. Fetch all existing role-permission relationships
	permRows, err := db.Query("SELECT role_id, permission FROM role_permission WHERE type = 0")
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to fetch permissions: " + err.Error()})
		c.Abort()
		return
	}
	defer permRows.Close()

	// Store existing permissions in a map for quick lookup
	existingPerms := make(map[string]map[string]bool) // permission -> roleName -> true
	for permRows.Next() {
		var roleID int
		var permission string
		if err := permRows.Scan(&roleID, &permission); err != nil {
			continue // Skip bad rows
		}
		roleName, ok := roleMap[roleID]
		if !ok {
			continue // Skip if role ID is invalid
		}
		if _, ok := existingPerms[permission]; !ok {
			existingPerms[permission] = make(map[string]bool)
		}
		existingPerms[permission][roleName] = true
	}

	// 3. Build the full matrix using Router.AllRoutes as the source of truth for rows
	permissionMatrix := make(map[string]map[string]bool)
	for _, route := range Router.AllRoutes {
		permission := route.Path
		permissionMatrix[permission] = make(map[string]bool)
		for _, role := range roles {
			// Check if this specific permission exists for this role
			if rolesWithPerm, ok := existingPerms[permission]; ok {
				if rolesWithPerm[role.Name] {
					permissionMatrix[permission][role.Name] = true
				} else {
					permissionMatrix[permission][role.Name] = false
				}
			} else {
				permissionMatrix[permission][role.Name] = false
			}
		}
	}

	// 4. Format the response to include the list of roles for the columns
	response := gin.H{
		"roles":     roles,
		"matrix":    permissionMatrix,
		"endpoints": Router.AllRoutes,
	}

	c.JSON(http.StatusOK, response)
}

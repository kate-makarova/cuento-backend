package Services

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Router"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

type PermissionType int

const (
	EndpointPermission PermissionType = 0
	SubforumPermission PermissionType = 1
)

var SubforumPermissions = map[string]string{
	"subforum_read":                   "View subforum",
	"subforum_create_general_topic":   "Create general topic",
	"subforum_create_episode_topic":   "Create episode topic",
	"subforum_create_character_topic": "Create character topic",
	"subforum_post":                   "Post in subforum",
	"subforum_delete_topic":           "Delete own topic",
	"subforum_delete_others_topic":    "Delete others' topic",
	"subforum_edit_others_post":       "Edit others' post",
	"subforum_edit_own_post":          "Edit own post",
}

type PermissionMatrixObject struct {
	Roles           map[int]string          `json:"roles"`
	Permissions     map[string]string       `json:"permissions"`
	Matrix          map[string]map[int]bool `json:"matrix"`
	PermissionOrder []string                `json:"permission_order"`
}

func GetEndpointPermissionMatrix(db *sql.DB) (PermissionMatrixObject, error) {
	// 1. Fetch all roles
	roleRows, err := db.Query("SELECT id, name FROM roles")
	if err != nil {
		return PermissionMatrixObject{}, err
	}
	defer roleRows.Close()

	roleMap := make(map[int]string)
	for roleRows.Next() {
		var role Entities.Role
		if err := roleRows.Scan(&role.Id, &role.Name); err != nil {
			return PermissionMatrixObject{}, err
		}
		roleMap[role.Id] = role.Name
	}

	// 2. Fetch all existing role-permission relationships
	permRows, err := db.Query("SELECT role_id, permission FROM role_permission WHERE type = 0")
	if err != nil {
		return PermissionMatrixObject{}, err
	}
	defer permRows.Close()

	existingPerms := make(map[string]map[int]bool) // permission -> roleID -> true
	for permRows.Next() {
		var roleID int
		var permission string
		if err := permRows.Scan(&roleID, &permission); err != nil {
			continue
		}
		if _, ok := roleMap[roleID]; ok {
			if _, ok := existingPerms[permission]; !ok {
				existingPerms[permission] = make(map[int]bool)
			}
			existingPerms[permission][roleID] = true
		}
	}

	// 3. Build the full matrix, permissions map, and ordered list of keys
	permissionMatrix := make(map[string]map[int]bool)
	permissionsMap := make(map[string]string)
	permissionOrder := make([]string, len(Router.AllRoutes))

	for i, route := range Router.AllRoutes {
		permission := route.Path
		permissionOrder[i] = permission
		permissionsMap[permission] = route.Definition
		permissionMatrix[permission] = make(map[int]bool)
		for roleID := range roleMap {
			if rolesWithPerm, ok := existingPerms[permission]; ok {
				permissionMatrix[permission][roleID] = rolesWithPerm[roleID]
			} else {
				permissionMatrix[permission][roleID] = false
			}
		}
	}

	return PermissionMatrixObject{
		Roles:           roleMap,
		Permissions:     permissionsMap,
		Matrix:          permissionMatrix,
		PermissionOrder: permissionOrder,
	}, nil
}

func GetSubforumPermissionMatrix(db *sql.DB) (PermissionMatrixObject, error) {
	// 1. Fetch all roles
	roleRows, err := db.Query("SELECT id, name FROM roles")
	if err != nil {
		return PermissionMatrixObject{}, err
	}
	defer roleRows.Close()

	roleMap := make(map[int]string)
	for roleRows.Next() {
		var role Entities.Role
		if err := roleRows.Scan(&role.Id, &role.Name); err != nil {
			return PermissionMatrixObject{}, err
		}
		roleMap[role.Id] = role.Name
	}

	// 2. Fetch all subforums
	subforumRows, err := db.Query("SELECT id, name FROM subforums")
	if err != nil {
		return PermissionMatrixObject{}, err
	}
	defer subforumRows.Close()

	type SubforumInfo struct {
		ID   int
		Name string
	}
	var subforums []SubforumInfo
	for subforumRows.Next() {
		var sub SubforumInfo
		if err := subforumRows.Scan(&sub.ID, &sub.Name); err != nil {
			return PermissionMatrixObject{}, err
		}
		subforums = append(subforums, sub)
	}

	// 3. Fetch all existing subforum role-permission relationships
	permRows, err := db.Query("SELECT role_id, permission FROM role_permission WHERE type = 1")
	if err != nil {
		return PermissionMatrixObject{}, err
	}
	defer permRows.Close()

	existingPerms := make(map[string]map[int]bool) // permission -> roleID -> true
	for permRows.Next() {
		var roleID int
		var permission string
		if err := permRows.Scan(&roleID, &permission); err != nil {
			continue
		}
		if _, ok := roleMap[roleID]; ok {
			if _, ok := existingPerms[permission]; !ok {
				existingPerms[permission] = make(map[int]bool)
			}
			existingPerms[permission][roleID] = true
		}
	}

	// 4. Build the matrix, permissions map, and ordered list of keys
	permissionMatrix := make(map[string]map[int]bool)
	allPossiblePerms := make(map[string]string)
	permissionOrder := make([]string, 0)

	for _, sub := range subforums {
		for permKey, permDef := range SubforumPermissions {
			permissionString := fmt.Sprintf("%s:%d", permKey, sub.ID)
			humanReadableDef := fmt.Sprintf("Subforum '%s' (ID %d): %s", sub.Name, sub.ID, permDef)

			permissionOrder = append(permissionOrder, permissionString)
			allPossiblePerms[permissionString] = humanReadableDef

			permissionMatrix[permissionString] = make(map[int]bool)
			for roleID := range roleMap {
				if rolesWithPerm, ok := existingPerms[permissionString]; ok {
					permissionMatrix[permissionString][roleID] = rolesWithPerm[roleID]
				} else {
					permissionMatrix[permissionString][roleID] = false
				}
			}
		}
	}

	return PermissionMatrixObject{
		Roles:           roleMap,
		Permissions:     allPossiblePerms,
		Matrix:          permissionMatrix,
		PermissionOrder: permissionOrder,
	}, nil
}

func UpdatePermissionMatrix(permissions []string, db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback() // Rollback on error

	// 1. Get all roles to map names to IDs
	roleRows, err := tx.Query("SELECT id, name FROM roles")
	if err != nil {
		return fmt.Errorf("failed to fetch roles: %w", err)
	}
	defer roleRows.Close()

	roleNameToID := make(map[string]int)
	for roleRows.Next() {
		var role Entities.Role
		if err := roleRows.Scan(&role.Id, &role.Name); err != nil {
			return fmt.Errorf("failed to scan role: %w", err)
		}
		roleNameToID[role.Name] = role.Id
	}

	// 2. Wipe all old permissions
	if _, err := tx.Exec("DELETE FROM role_permission"); err != nil {
		return fmt.Errorf("failed to delete old permissions: %w", err)
	}

	// 3. Prepare for bulk insert
	stmt, err := tx.Prepare("INSERT INTO role_permission (type, role_id, permission) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer stmt.Close()

	// 4. Parse and insert new permissions
	for _, p := range permissions {
		parts := strings.SplitN(p, ".", 3)
		if len(parts) != 3 {
			continue // Skip malformed strings
		}

		permType, err := strconv.Atoi(parts[0])
		if err != nil {
			continue // Skip if type is not a number
		}

		roleName := parts[1]
		roleID, ok := roleNameToID[roleName]
		if !ok {
			continue // Skip if role name is invalid
		}

		permission := parts[2]

		if _, err := stmt.Exec(permType, roleID, permission); err != nil {
			return fmt.Errorf("failed to insert permission '%s': %w", p, err)
		}
	}

	// 5. Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

package Services

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Router"
	"database/sql"
	"fmt"
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
	Roles       map[int]string          `json:"roles"`
	Permissions map[string]string       `json:"permissions"`
	Matrix      map[string]map[int]bool `json:"matrix"`
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
		if _, ok := roleMap[roleID]; ok { // Ensure role exists
			if _, ok := existingPerms[permission]; !ok {
				existingPerms[permission] = make(map[int]bool)
			}
			existingPerms[permission][roleID] = true
		}
	}

	// 3. Build the full matrix and permissions map
	permissionMatrix := make(map[string]map[int]bool)
	permissionsMap := make(map[string]string)
	for _, route := range Router.AllRoutes {
		permission := route.Path
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
		Roles:       roleMap,
		Permissions: permissionsMap,
		Matrix:      permissionMatrix,
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

	// 4. Build the matrix and permissions map, grouped by subforum
	permissionMatrix := make(map[string]map[int]bool)
	allPossiblePerms := make(map[string]string)

	for _, sub := range subforums {
		for permKey, permDef := range SubforumPermissions {
			permissionString := fmt.Sprintf("%s:%d", permKey, sub.ID)
			humanReadableDef := fmt.Sprintf("Subforum '%s' (ID %d): %s", sub.Name, sub.ID, permDef)
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
		Roles:       roleMap,
		Permissions: allPossiblePerms,
		Matrix:      permissionMatrix,
	}, nil
}

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
	Roles       map[int]string
	Permissions map[string]string
	Matrix      map[string]map[string]bool
}

func GetEndpointPermissionMatrix(db *sql.DB) (PermissionMatrixObject, error) {
	// 1. Fetch all roles
	roleRows, err := db.Query("SELECT id, name FROM roles")
	if err != nil {
		return PermissionMatrixObject{}, err
	}
	defer roleRows.Close()

	roles := make([]Entities.Role, 0)
	roleMap := make(map[int]string)
	for roleRows.Next() {
		var role Entities.Role
		if err := roleRows.Scan(&role.Id, &role.Name); err != nil {
			return PermissionMatrixObject{}, err
		}
		roles = append(roles, role)
		roleMap[role.Id] = role.Name
	}

	// 2. Fetch all existing role-permission relationships
	permRows, err := db.Query("SELECT role_id, permission FROM role_permission WHERE type = 0")
	if err != nil {
		return PermissionMatrixObject{}, err
	}
	defer permRows.Close()

	existingPerms := make(map[string]map[string]bool) // permission -> roleName -> true
	for permRows.Next() {
		var roleID int
		var permission string
		if err := permRows.Scan(&roleID, &permission); err != nil {
			continue
		}
		roleName, ok := roleMap[roleID]
		if !ok {
			continue
		}
		if _, ok := existingPerms[permission]; !ok {
			existingPerms[permission] = make(map[string]bool)
		}
		existingPerms[permission][roleName] = true
	}

	// 3. Build the full matrix and permissions map
	permissionMatrix := make(map[string]map[string]bool)
	permissionsMap := make(map[string]string)
	for _, route := range Router.AllRoutes {
		permission := route.Path
		permissionsMap[permission] = route.Definition // Use the route definition
		permissionMatrix[permission] = make(map[string]bool)
		for _, role := range roles {
			if rolesWithPerm, ok := existingPerms[permission]; ok {
				permissionMatrix[permission][role.Name] = rolesWithPerm[role.Name]
			} else {
				permissionMatrix[permission][role.Name] = false
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

	roles := make([]Entities.Role, 0)
	roleMap := make(map[int]string)
	for roleRows.Next() {
		var role Entities.Role
		if err := roleRows.Scan(&role.Id, &role.Name); err != nil {
			return PermissionMatrixObject{}, err
		}
		roles = append(roles, role)
		roleMap[role.Id] = role.Name
	}

	// 2. Fetch all subforums
	subforumRows, err := db.Query("SELECT id FROM subforums")
	if err != nil {
		return PermissionMatrixObject{}, err
	}
	defer subforumRows.Close()

	var subforumIDs []int
	for subforumRows.Next() {
		var id int
		if err := subforumRows.Scan(&id); err != nil {
			return PermissionMatrixObject{}, err
		}
		subforumIDs = append(subforumIDs, id)
	}

	// 3. Generate all possible permission strings and their definitions
	allPossiblePerms := make(map[string]string)
	for _, subID := range subforumIDs {
		for permKey, permDef := range SubforumPermissions {
			allPossiblePerms[fmt.Sprintf("%s:%d", permKey, subID)] = permDef
		}
	}

	// 4. Fetch all existing subforum role-permission relationships
	permRows, err := db.Query("SELECT role_id, permission FROM role_permission WHERE type = 1")
	if err != nil {
		return PermissionMatrixObject{}, err
	}
	defer permRows.Close()

	existingPerms := make(map[string]map[string]bool) // permission -> roleName -> true
	for permRows.Next() {
		var roleID int
		var permission string
		if err := permRows.Scan(&roleID, &permission); err != nil {
			continue
		}
		roleName, ok := roleMap[roleID]
		if !ok {
			continue
		}
		if _, ok := existingPerms[permission]; !ok {
			existingPerms[permission] = make(map[string]bool)
		}
		existingPerms[permission][roleName] = true
	}

	// 5. Build the full matrix
	permissionMatrix := make(map[string]map[string]bool)
	for permission := range allPossiblePerms {
		permissionMatrix[permission] = make(map[string]bool)
		for _, role := range roles {
			if rolesWithPerm, ok := existingPerms[permission]; ok {
				permissionMatrix[permission][role.Name] = rolesWithPerm[role.Name]
			} else {
				permissionMatrix[permission][role.Name] = false
			}
		}
	}

	return PermissionMatrixObject{
		Roles:       roleMap,
		Permissions: allPossiblePerms,
		Matrix:      permissionMatrix,
	}, nil
}

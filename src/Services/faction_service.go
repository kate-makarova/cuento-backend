package Services

import (
	"cuento-backend/src/Entities"
	"database/sql"
	"sort"
)

func GetFactionTreeByRoot(rootID int, db *sql.DB) ([]Entities.Faction, error) {
	// Fetch all factions that belong to this root (including the root itself)
	query := `
		SELECT id, name, parent_id, level, description, icon, show_on_profile 
		FROM factions 
		WHERE root_id = ? OR id = ?
	`
	rows, err := db.Query(query, rootID, rootID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var allFactions []Entities.Faction
	for rows.Next() {
		var f Entities.Faction
		if err := rows.Scan(&f.Id, &f.Name, &f.ParentId, &f.Level, &f.Description, &f.Icon, &f.ShowOnProfile); err != nil {
			return nil, err
		}
		allFactions = append(allFactions, f)
	}

	// Build adjacency list
	childrenMap := make(map[int][]Entities.Faction)
	var root Entities.Faction
	var rootFound bool

	for _, f := range allFactions {
		if f.Id == rootID {
			root = f
			rootFound = true
		}
		if f.ParentId != nil {
			childrenMap[*f.ParentId] = append(childrenMap[*f.ParentId], f)
		}
	}

	if !rootFound {
		return []Entities.Faction{}, nil
	}

	// Sort children by name to ensure deterministic order
	for parentID := range childrenMap {
		sort.Slice(childrenMap[parentID], func(i, j int) bool {
			return childrenMap[parentID][i].Name < childrenMap[parentID][j].Name
		})
	}

	// DFS to flatten the tree in pre-order traversal
	var result []Entities.Faction
	var dfs func(int)
	dfs = func(parentID int) {
		if children, ok := childrenMap[parentID]; ok {
			for _, child := range children {
				result = append(result, child)
				dfs(child.Id)
			}
		}
	}

	result = append(result, root)
	dfs(root.Id)

	return result, nil
}

func GetFactionTree(db *sql.DB) ([]Entities.Faction, error) {
	// Fetch all factions
	query := `
		SELECT id, name, parent_id, level, description, icon, show_on_profile 
		FROM factions
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var allFactions []Entities.Faction
	for rows.Next() {
		var f Entities.Faction
		if err := rows.Scan(&f.Id, &f.Name, &f.ParentId, &f.Level, &f.Description, &f.Icon, &f.ShowOnProfile); err != nil {
			return nil, err
		}
		allFactions = append(allFactions, f)
	}

	// Build adjacency list and identify roots
	childrenMap := make(map[int][]Entities.Faction)
	var roots []Entities.Faction

	for _, f := range allFactions {
		if f.ParentId == nil {
			roots = append(roots, f)
		} else {
			childrenMap[*f.ParentId] = append(childrenMap[*f.ParentId], f)
		}
	}

	// Sort roots by name
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].Name < roots[j].Name
	})

	// Sort children by name
	for parentID := range childrenMap {
		sort.Slice(childrenMap[parentID], func(i, j int) bool {
			return childrenMap[parentID][i].Name < childrenMap[parentID][j].Name
		})
	}

	// DFS to flatten the tree
	var result []Entities.Faction
	var dfs func(int)
	dfs = func(parentID int) {
		if children, ok := childrenMap[parentID]; ok {
			for _, child := range children {
				result = append(result, child)
				dfs(child.Id)
			}
		}
	}

	for _, root := range roots {
		result = append(result, root)
		dfs(root.Id)
	}

	return result, nil
}

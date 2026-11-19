package Services

import (
	"cuento-backend/src/Entities"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInventoryNotEnabled    = errors.New("inventory is not enabled for this arc")
	ErrInventoryInUse         = errors.New("inventory is already used in a scene of this arc")
	ErrInventoryItemInUse     = errors.New("this item is in a locked inventory; hide it instead")
	ErrInventoryLocked        = errors.New("this inventory is locked")
	ErrSceneHasLockedInv      = errors.New("this scene already has locked inventories")
	ErrSceneNotUsingInventory = errors.New("this scene does not use inventory")
	ErrScenePending           = errors.New("this scene has not started yet")
	ErrSceneNotPending        = errors.New("this scene is not pending")
	ErrInventoriesUnlocked    = errors.New("some inventories are not locked")
	ErrProfileNotInScene      = errors.New("this character is not in the scene")
	ErrInvalidInventoryItems  = errors.New("items must belong to the arc, appear once and have a positive quantity")
)

// ArcInventoryState reports whether inventory is enabled for the arc and whether any of its scenes uses it.
func ArcInventoryState(db *sql.DB, arcID int) (enabled, inUse bool, err error) {
	err = db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM inventory_arc_settings WHERE arc_id = ?),
		       EXISTS(SELECT 1 FROM inventory_episode_settings s JOIN arc_episodes ae ON ae.episode_id = s.episode_id WHERE ae.arc_id = ?)`,
		arcID, arcID,
	).Scan(&enabled, &inUse)
	if err != nil {
		return false, false, fmt.Errorf("loading inventory state of arc %d: %w", arcID, err)
	}
	return enabled, inUse, nil
}

func EnableArcInventory(db *sql.DB, arcID int) error {
	if _, err := db.Exec("INSERT IGNORE INTO inventory_arc_settings (arc_id, enabled_at) VALUES (?, NOW())", arcID); err != nil {
		return fmt.Errorf("enabling inventory for arc %d: %w", arcID, err)
	}
	return nil
}

// DisableArcInventory turns inventory off; refused once any scene of the arc uses it.
func DisableArcInventory(db *sql.DB, arcID int) error {
	_, inUse, err := ArcInventoryState(db, arcID)
	if err != nil {
		return err
	}
	if inUse {
		return ErrInventoryInUse
	}
	if _, err := db.Exec("DELETE FROM inventory_arc_settings WHERE arc_id = ?", arcID); err != nil {
		return fmt.Errorf("disabling inventory for arc %d: %w", arcID, err)
	}
	return nil
}

const inventoryItemColumns = "id, arc_id, name, description, icon, is_hidden, display_order"

func scanInventoryItem(row interface{ Scan(...any) error }) (Entities.InventoryItem, error) {
	var it Entities.InventoryItem
	var description, icon sql.NullString
	if err := row.Scan(&it.ID, &it.ArcID, &it.Name, &description, &icon, &it.IsHidden, &it.DisplayOrder); err != nil {
		return it, err
	}
	if description.Valid {
		it.Description = &description.String
	}
	if icon.Valid {
		it.Icon = &icon.String
	}
	return it, nil
}

// ListArcInventoryItems returns the arc's catalogue; hidden items only when includeHidden is set.
func ListArcInventoryItems(db *sql.DB, arcID int, includeHidden bool) ([]Entities.InventoryItem, error) {
	query := "SELECT " + inventoryItemColumns + " FROM inventory_items WHERE arc_id = ?"
	if !includeHidden {
		query += " AND is_hidden = 0"
	}
	rows, err := db.Query(query+" ORDER BY display_order, id", arcID)
	if err != nil {
		return nil, fmt.Errorf("listing inventory items of arc %d: %w", arcID, err)
	}
	defer rows.Close()

	items := []Entities.InventoryItem{}
	for rows.Next() {
		it, err := scanInventoryItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning inventory item: %w", err)
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// GetInventoryItem returns sql.ErrNoRows when the item does not exist.
func GetInventoryItem(db *sql.DB, id int) (Entities.InventoryItem, error) {
	return scanInventoryItem(db.QueryRow("SELECT "+inventoryItemColumns+" FROM inventory_items WHERE id = ?", id))
}

func CreateInventoryItem(db *sql.DB, it Entities.InventoryItem) (int, error) {
	enabled, _, err := ArcInventoryState(db, it.ArcID)
	if err != nil {
		return 0, err
	}
	if !enabled {
		return 0, ErrInventoryNotEnabled
	}
	res, err := db.Exec(
		"INSERT INTO inventory_items (arc_id, name, description, icon, is_hidden, display_order) VALUES (?, ?, ?, ?, ?, ?)",
		it.ArcID, it.Name, it.Description, it.Icon, it.IsHidden, it.DisplayOrder,
	)
	if err != nil {
		return 0, fmt.Errorf("creating inventory item in arc %d: %w", it.ArcID, err)
	}
	id, err := res.LastInsertId()
	return int(id), err
}

type InventoryItemUpdate struct {
	Name         *string
	Description  *string
	Icon         *string
	IsHidden     *bool
	DisplayOrder *int
}

func UpdateInventoryItem(db *sql.DB, id int, u InventoryItemUpdate) error {
	var sets []string
	var args []interface{}
	add := func(column string, value interface{}) {
		sets = append(sets, column+" = ?")
		args = append(args, value)
	}
	if u.Name != nil {
		add("name", *u.Name)
	}
	if u.Description != nil {
		add("description", *u.Description)
	}
	if u.Icon != nil {
		add("icon", *u.Icon)
	}
	if u.IsHidden != nil {
		add("is_hidden", *u.IsHidden)
	}
	if u.DisplayOrder != nil {
		add("display_order", *u.DisplayOrder)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, id)
	if _, err := db.Exec("UPDATE inventory_items SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil {
		return fmt.Errorf("updating inventory item %d: %w", id, err)
	}
	return nil
}

// DeleteInventoryItem removes an item; refused while it is in a locked inventory (it would rewrite history).
func DeleteInventoryItem(db *sql.DB, id int) error {
	var used bool
	if err := db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM inventory_scene_items si JOIN inventory_scene s ON s.id = si.scene_inventory_id
		              WHERE si.item_id = ? AND s.is_locked = 1)`, id,
	).Scan(&used); err != nil {
		return fmt.Errorf("checking use of inventory item %d: %w", id, err)
	}
	if used {
		return ErrInventoryItemInUse
	}
	if _, err := db.Exec("DELETE FROM inventory_items WHERE id = ?", id); err != nil {
		return fmt.Errorf("deleting inventory item %d: %w", id, err)
	}
	return nil
}

// SceneInventoryContext is what the inventory endpoints need to know about a scene.
type SceneInventoryContext struct {
	EpisodeID     int
	Name          string
	TopicID       int64
	SubforumID    int
	Status        Entities.EpisodeStatus
	ArcID         int // 0 when the scene is not in an arc
	UsesInventory bool
}

// GetSceneInventoryContext returns sql.ErrNoRows when the scene does not exist.
func GetSceneInventoryContext(db *sql.DB, episodeID int) (SceneInventoryContext, error) {
	ctx := SceneInventoryContext{EpisodeID: episodeID}
	var arcID sql.NullInt64
	err := db.QueryRow(`
		SELECT e.name, t.id, t.subforum_id, e.episode_status,
		       (SELECT ae.arc_id FROM arc_episodes ae WHERE ae.episode_id = e.id LIMIT 1),
		       EXISTS(SELECT 1 FROM inventory_episode_settings s WHERE s.episode_id = e.id)
		FROM episode_base e JOIN topics t ON t.id = e.topic_id
		WHERE e.id = ? AND t.status != ?`,
		episodeID, Entities.DeletedTopic,
	).Scan(&ctx.Name, &ctx.TopicID, &ctx.SubforumID, &ctx.Status, &arcID, &ctx.UsesInventory)
	if err != nil {
		return ctx, err
	}
	ctx.ArcID = int(arcID.Int64)
	return ctx, nil
}

// EnableSceneInventoryTx marks a new scene as using inventory and puts it into Pending.
func EnableSceneInventoryTx(tx *sql.Tx, episodeID int) error {
	if _, err := tx.Exec("INSERT INTO inventory_episode_settings (episode_id) VALUES (?)", episodeID); err != nil {
		return fmt.Errorf("enabling inventory for scene %d: %w", episodeID, err)
	}
	if _, err := tx.Exec("UPDATE episode_base SET episode_status = ? WHERE id = ?", Entities.PendingEpisode, episodeID); err != nil {
		return fmt.Errorf("making scene %d pending: %w", episodeID, err)
	}
	return nil
}

// SetSceneUsesInventory switches inventory on or off for an existing scene.
// Off is refused while the scene is pending or once any of its inventories is locked.
func SetSceneUsesInventory(db *sql.DB, ctx SceneInventoryContext, on bool) error {
	if on {
		enabled, _, err := ArcInventoryState(db, ctx.ArcID)
		if err != nil {
			return err
		}
		if ctx.ArcID == 0 || !enabled {
			return ErrInventoryNotEnabled
		}
		if _, err := db.Exec("INSERT IGNORE INTO inventory_episode_settings (episode_id) VALUES (?)", ctx.EpisodeID); err != nil {
			return fmt.Errorf("enabling inventory for scene %d: %w", ctx.EpisodeID, err)
		}
		return nil
	}

	if ctx.Status == Entities.PendingEpisode {
		return ErrScenePending
	}
	var locked bool
	if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM inventory_scene WHERE episode_id = ? AND is_locked = 1)", ctx.EpisodeID).Scan(&locked); err != nil {
		return fmt.Errorf("checking locked inventories of scene %d: %w", ctx.EpisodeID, err)
	}
	if locked {
		return ErrSceneHasLockedInv
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM inventory_scene WHERE episode_id = ?", ctx.EpisodeID); err != nil {
		return fmt.Errorf("removing draft inventories of scene %d: %w", ctx.EpisodeID, err)
	}
	if _, err := tx.Exec("DELETE FROM inventory_episode_settings WHERE episode_id = ?", ctx.EpisodeID); err != nil {
		return fmt.Errorf("disabling inventory for scene %d: %w", ctx.EpisodeID, err)
	}
	return tx.Commit()
}

// sceneProfilesQuery lists the profiles taking part in a scene: each character's main profile and each mask.
const sceneProfilesQuery = `
	SELECT cpb.id, cb.name, FALSE, cb.user_id, u.username
	FROM episode_character ec
	JOIN character_base cb ON cb.id = ec.character_id
	JOIN character_profile_base cpb ON cpb.character_id = cb.id AND cpb.is_mask IS NOT TRUE
	JOIN users u ON u.id = cb.user_id
	WHERE ec.episode_id = ?
	UNION ALL
	SELECT cpb.id, cpb.mask_name, TRUE, cpb.user_id, u.username
	FROM episode_mask em
	JOIN character_profile_base cpb ON cpb.id = em.mask_id
	JOIN users u ON u.id = cpb.user_id
	WHERE em.episode_id = ?`

// GetSceneInventories returns every profile of the scene with its starting inventory and lock state.
func GetSceneInventories(db *sql.DB, episodeID int) ([]Entities.SceneInventory, error) {
	rows, err := db.Query(sceneProfilesQuery, episodeID, episodeID)
	if err != nil {
		return nil, fmt.Errorf("listing profiles of scene %d: %w", episodeID, err)
	}
	defer rows.Close()

	list := []Entities.SceneInventory{}
	index := map[int]int{}
	for rows.Next() {
		var inv Entities.SceneInventory
		var name sql.NullString
		if err := rows.Scan(&inv.ProfileID, &name, &inv.IsMask, &inv.OwnerUserID, &inv.OwnerUsername); err != nil {
			return nil, fmt.Errorf("scanning scene profile: %w", err)
		}
		if _, dup := index[inv.ProfileID]; dup {
			continue
		}
		inv.Name = name.String
		inv.Items = []Entities.InventoryEntry{}
		index[inv.ProfileID] = len(list)
		list = append(list, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	stateRows, err := db.Query("SELECT character_profile_id, is_locked, locked_at FROM inventory_scene WHERE episode_id = ?", episodeID)
	if err != nil {
		return nil, fmt.Errorf("loading inventory state of scene %d: %w", episodeID, err)
	}
	defer stateRows.Close()
	for stateRows.Next() {
		var profileID int
		var locked bool
		var lockedAt sql.NullTime
		if err := stateRows.Scan(&profileID, &locked, &lockedAt); err != nil {
			return nil, fmt.Errorf("scanning inventory state: %w", err)
		}
		if i, ok := index[profileID]; ok {
			list[i].Exists = true
			list[i].IsLocked = locked
			if lockedAt.Valid {
				list[i].LockedAt = &lockedAt.Time
			}
		}
	}
	if err := stateRows.Err(); err != nil {
		return nil, err
	}

	err = fillInventoryEntries(db, list, index, `
		SELECT s.character_profile_id, i.id, i.name, i.description, i.icon, si.quantity
		FROM inventory_scene_items si
		JOIN inventory_scene s ON s.id = si.scene_inventory_id
		JOIN inventory_items i ON i.id = si.item_id
		WHERE s.episode_id = ?
		ORDER BY i.display_order, i.id`, episodeID)
	return list, err
}

// fillInventoryEntries runs a query yielding (profile_id, item columns, quantity) and appends the entries.
func fillInventoryEntries(db *sql.DB, list []Entities.SceneInventory, index map[int]int, query string, args ...interface{}) error {
	rows, err := db.Query(query, args...)
	if err != nil {
		return fmt.Errorf("loading inventory items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var profileID int
		e, err := scanInventoryEntry(rows, &profileID)
		if err != nil {
			return err
		}
		if i, ok := index[profileID]; ok {
			list[i].Items = append(list[i].Items, e)
		}
	}
	return rows.Err()
}

// scanInventoryEntry scans (leading..., item id, name, description, icon, quantity).
func scanInventoryEntry(row interface{ Scan(...any) error }, leading ...any) (Entities.InventoryEntry, error) {
	var e Entities.InventoryEntry
	var description, icon sql.NullString
	if err := row.Scan(append(leading, &e.ItemID, &e.Name, &description, &icon, &e.Quantity)...); err != nil {
		return e, fmt.Errorf("scanning inventory entry: %w", err)
	}
	if description.Valid {
		e.Description = &description.String
	}
	if icon.Valid {
		e.Icon = &icon.String
	}
	return e, nil
}

// GetCurrentSceneInventories is GetSceneInventories with each locked inventory replaced by its latest post copy.
func GetCurrentSceneInventories(db *sql.DB, episodeID int) ([]Entities.SceneInventory, error) {
	list, err := GetSceneInventories(db, episodeID)
	if err != nil {
		return nil, err
	}
	index := map[int]int{}
	for i, inv := range list {
		index[inv.ProfileID] = i
	}

	rows, err := db.Query(`
		SELECT s.character_profile_id
		FROM inventory_scene s
		WHERE s.episode_id = ? AND EXISTS(SELECT 1 FROM inventory_post p WHERE p.scene_inventory_id = s.id)`, episodeID)
	if err != nil {
		return nil, fmt.Errorf("finding post copies of scene %d: %w", episodeID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var profileID int
		if err := rows.Scan(&profileID); err != nil {
			return nil, fmt.Errorf("scanning post copy: %w", err)
		}
		if i, ok := index[profileID]; ok {
			list[i].Items = []Entities.InventoryEntry{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	err = fillInventoryEntries(db, list, index, `
		SELECT s.character_profile_id, i.id, i.name, i.description, i.icon, pi.quantity
		FROM inventory_scene s
		JOIN inventory_post_items pi ON pi.post_id = (SELECT MAX(p.post_id) FROM inventory_post p WHERE p.scene_inventory_id = s.id)
		JOIN inventory_items i ON i.id = pi.item_id
		WHERE s.episode_id = ?
		ORDER BY i.display_order, i.id`, episodeID)
	return list, err
}

func profileInScene(db *sql.DB, episodeID, profileID int) error {
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM ("+sceneProfilesQuery+") p WHERE p.id = ?", episodeID, episodeID, profileID).Scan(&n); err != nil {
		return fmt.Errorf("checking profile %d in scene %d: %w", profileID, episodeID, err)
	}
	if n == 0 {
		return ErrProfileNotInScene
	}
	return nil
}

type InventoryQuantity struct {
	ItemID   int `json:"item_id"`
	Quantity int `json:"quantity"`
}

// ValidateInventoryQuantities checks that each item appears once with a positive quantity.
func ValidateInventoryQuantities(items []InventoryQuantity) error {
	seen := map[int]bool{}
	for _, it := range items {
		if it.Quantity <= 0 || seen[it.ItemID] {
			return ErrInvalidInventoryItems
		}
		seen[it.ItemID] = true
	}
	return nil
}

// SetSceneInventoryItems replaces the starting inventory of a profile in a scene; refused once locked.
func SetSceneInventoryItems(db *sql.DB, ctx SceneInventoryContext, profileID int, items []InventoryQuantity) error {
	if err := ValidateInventoryQuantities(items); err != nil {
		return err
	}
	if err := profileInScene(db, ctx.EpisodeID, profileID); err != nil {
		return err
	}
	if len(items) > 0 {
		placeholders := make([]string, len(items))
		args := []interface{}{ctx.ArcID}
		for i, it := range items {
			placeholders[i] = "?"
			args = append(args, it.ItemID)
		}
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM inventory_items WHERE arc_id = ? AND id IN ("+strings.Join(placeholders, ",")+")", args...).Scan(&n); err != nil {
			return fmt.Errorf("checking inventory items: %w", err)
		}
		if n != len(items) {
			return ErrInvalidInventoryItems
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	sceneInvID, locked, err := ensureSceneInventory(tx, ctx.EpisodeID, profileID)
	if err != nil {
		return err
	}
	if locked {
		return ErrInventoryLocked
	}
	if _, err := tx.Exec("DELETE FROM inventory_scene_items WHERE scene_inventory_id = ?", sceneInvID); err != nil {
		return fmt.Errorf("clearing inventory %d: %w", sceneInvID, err)
	}
	for _, it := range items {
		if _, err := tx.Exec("INSERT INTO inventory_scene_items (scene_inventory_id, item_id, quantity) VALUES (?, ?, ?)", sceneInvID, it.ItemID, it.Quantity); err != nil {
			return fmt.Errorf("adding item %d to inventory %d: %w", it.ItemID, sceneInvID, err)
		}
	}
	return tx.Commit()
}

// ensureSceneInventory returns the scene inventory row of a profile, creating an empty one, locked for update.
func ensureSceneInventory(tx *sql.Tx, episodeID, profileID int) (id int, locked bool, err error) {
	if _, err := tx.Exec("INSERT IGNORE INTO inventory_scene (episode_id, character_profile_id) VALUES (?, ?)", episodeID, profileID); err != nil {
		return 0, false, fmt.Errorf("creating inventory for profile %d in scene %d: %w", profileID, episodeID, err)
	}
	err = tx.QueryRow("SELECT id, is_locked FROM inventory_scene WHERE episode_id = ? AND character_profile_id = ? FOR UPDATE", episodeID, profileID).Scan(&id, &locked)
	if err != nil {
		return 0, false, fmt.Errorf("loading inventory for profile %d in scene %d: %w", profileID, episodeID, err)
	}
	return id, locked, nil
}

// LockSceneInventory locks a profile's inventory (creating an empty one if none exists). Locking twice is a no-op.
func LockSceneInventory(db *sql.DB, ctx SceneInventoryContext, profileID, userID int) error {
	if err := profileInScene(db, ctx.EpisodeID, profileID); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	id, locked, err := ensureSceneInventory(tx, ctx.EpisodeID, profileID)
	if err != nil {
		return err
	}
	if !locked {
		if _, err := tx.Exec("UPDATE inventory_scene SET is_locked = 1, locked_at = NOW(), locked_by = ? WHERE id = ?", userID, id); err != nil {
			return fmt.Errorf("locking inventory %d: %w", id, err)
		}
	}
	return tx.Commit()
}

// StartScene moves a pending scene to Active once every profile's inventory is locked.
// On ErrInventoriesUnlocked it also returns the names of the profiles still unlocked.
func StartScene(db *sql.DB, ctx SceneInventoryContext, userID int) ([]string, error) {
	if ctx.Status != Entities.PendingEpisode {
		return nil, ErrSceneNotPending
	}
	inventories, err := GetSceneInventories(db, ctx.EpisodeID)
	if err != nil {
		return nil, err
	}
	var unlocked []string
	for _, inv := range inventories {
		if !inv.IsLocked {
			unlocked = append(unlocked, inv.Name)
		}
	}
	if len(unlocked) > 0 {
		return unlocked, ErrInventoriesUnlocked
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()
	res, err := tx.Exec("UPDATE episode_base SET episode_status = ? WHERE id = ? AND episode_status = ?",
		Entities.ActiveEpisode, ctx.EpisodeID, Entities.PendingEpisode)
	if err != nil {
		return nil, fmt.Errorf("starting scene %d: %w", ctx.EpisodeID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrSceneNotPending
	}
	AddTopicActivityLog(tx, userID, ctx.TopicID, "episode_status_changed", int(Entities.PendingEpisode), int(Entities.ActiveEpisode))
	return nil, tx.Commit()
}

// SnapshotPostInventory stores the inventory a post's profile holds as of that post: a copy of its
// previous post's inventory, or of the starting inventory for its first post. Does nothing for posts
// whose profile has no locked inventory in the scene, and is safe to run twice for the same post.
func SnapshotPostInventory(db *sql.DB, postID int64) error {
	var sceneInvID int
	err := db.QueryRow(`
		SELECT s.id
		FROM posts p
		JOIN episode_base e ON e.topic_id = p.topic_id
		JOIN inventory_episode_settings es ON es.episode_id = e.id
		JOIN inventory_scene s ON s.episode_id = e.id AND s.character_profile_id = p.character_profile_id
		WHERE p.id = ? AND p.use_character_profile = 1 AND s.is_locked = 1`, postID,
	).Scan(&sceneInvID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("finding inventory for post %d: %w", postID, err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec("INSERT IGNORE INTO inventory_post (post_id, scene_inventory_id) VALUES (?, ?)", postID, sceneInvID)
	if err != nil {
		return fmt.Errorf("creating inventory copy for post %d: %w", postID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}

	var prevPostID sql.NullInt64
	if err := tx.QueryRow("SELECT MAX(post_id) FROM inventory_post WHERE scene_inventory_id = ? AND post_id < ?", sceneInvID, postID).Scan(&prevPostID); err != nil {
		return fmt.Errorf("finding previous inventory copy for post %d: %w", postID, err)
	}
	if prevPostID.Valid {
		_, err = tx.Exec("INSERT INTO inventory_post_items (post_id, item_id, quantity) SELECT ?, item_id, quantity FROM inventory_post_items WHERE post_id = ?",
			postID, prevPostID.Int64)
	} else {
		_, err = tx.Exec("INSERT INTO inventory_post_items (post_id, item_id, quantity) SELECT ?, item_id, quantity FROM inventory_scene_items WHERE scene_inventory_id = ?",
			postID, sceneInvID)
	}
	if err != nil {
		return fmt.Errorf("copying inventory items for post %d: %w", postID, err)
	}
	return tx.Commit()
}

// PostInventoryContext identifies whose inventory a post shows and where the post lives.
type PostInventoryContext struct {
	Scene       SceneInventoryContext
	ProfileID   int
	OwnerUserID int
	Name        string
}

// GetPostInventoryContext returns sql.ErrNoRows for a missing post or one not written as a character in a scene.
func GetPostInventoryContext(db *sql.DB, postID int) (PostInventoryContext, error) {
	var ctx PostInventoryContext
	var episodeID int
	var name sql.NullString
	err := db.QueryRow(`
		SELECT e.id, cpb.id, COALESCE(cb.user_id, cpb.user_id), IF(cpb.is_mask, cpb.mask_name, cb.name)
		FROM posts p
		JOIN episode_base e ON e.topic_id = p.topic_id
		JOIN character_profile_base cpb ON cpb.id = p.character_profile_id
		LEFT JOIN character_base cb ON cb.id = cpb.character_id
		WHERE p.id = ? AND p.use_character_profile = 1 AND p.is_deleted IS NOT TRUE`, postID,
	).Scan(&episodeID, &ctx.ProfileID, &ctx.OwnerUserID, &name)
	if err != nil {
		return ctx, err
	}
	ctx.Name = name.String
	ctx.Scene, err = GetSceneInventoryContext(db, episodeID)
	return ctx, err
}

// GetPostInventoryItems returns the inventory copy stored for a post; empty when the post has none.
func GetPostInventoryItems(db *sql.DB, postID int) ([]Entities.InventoryEntry, error) {
	rows, err := db.Query(`
		SELECT i.id, i.name, i.description, i.icon, pi.quantity
		FROM inventory_post_items pi JOIN inventory_items i ON i.id = pi.item_id
		WHERE pi.post_id = ?
		ORDER BY i.display_order, i.id`, postID)
	if err != nil {
		return nil, fmt.Errorf("loading inventory of post %d: %w", postID, err)
	}
	defer rows.Close()
	items := []Entities.InventoryEntry{}
	for rows.Next() {
		e, err := scanInventoryEntry(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, e)
	}
	return items, rows.Err()
}

// LocalizeSceneInventories fills the display string of each lock time.
func LocalizeSceneInventories(list []Entities.SceneInventory, locale UserLocale) {
	for i := range list {
		if list[i].LockedAt != nil {
			list[i].LockedAtLocalized = LocalizeTimeForUser(*list[i].LockedAt, locale)
		}
	}
}

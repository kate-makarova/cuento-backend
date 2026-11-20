package Services

import (
	"cuento-backend/src/Entities"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/frustra/bbcode"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// In-game inventory actions written in posts:
//   [item-drop items=12:1,15:3]text[/item-drop]           — the items leave the author's inventory
//   [item-give to=345 items=12:1]text[/item-give]         — the items move to another profile in the scene
// They are applied when the post is published; each post keeps a copy of every inventory it changed.

type InventoryActionKind string

const (
	InventoryDrop InventoryActionKind = "drop"
	InventoryGive InventoryActionKind = "give"
)

var inventoryActionTags = map[string]InventoryActionKind{
	"item-drop": InventoryDrop,
	"item-give": InventoryGive,
}

type InventoryAction struct {
	Kind        InventoryActionKind
	ToProfileID int // give only
	Items       []InventoryQuantity
}

// InventoryActionError is a problem with a post's inventory actions that its author can fix.
type InventoryActionError struct{ Message string }

func (e *InventoryActionError) Error() string { return e.Message }

func actionError(format string, args ...any) error {
	return &InventoryActionError{Message: fmt.Sprintf(format, args...)}
}

var ErrInventoryActionsFrozen = errors.New("inventory actions can only be changed in the latest post of the scene")

// String is the canonical form "kind;to;id:qty,...", also used as the rendered data-action attribute.
func (a InventoryAction) String() string {
	parts := make([]string, len(a.Items))
	for i, it := range a.Items {
		parts[i] = fmt.Sprintf("%d:%d", it.ItemID, it.Quantity)
	}
	return fmt.Sprintf("%s;%d;%s", a.Kind, a.ToProfileID, strings.Join(parts, ","))
}

func parseInventoryItemList(s string) ([]InventoryQuantity, error) {
	var items []InventoryQuantity
	for _, pair := range strings.Split(s, ",") {
		id, qty, ok := strings.Cut(strings.TrimSpace(pair), ":")
		itemID, err1 := strconv.Atoi(id)
		quantity, err2 := strconv.Atoi(qty)
		if !ok || err1 != nil || err2 != nil || itemID <= 0 {
			return nil, ErrInvalidInventoryItems
		}
		items = append(items, InventoryQuantity{ItemID: itemID, Quantity: quantity})
	}
	if err := ValidateInventoryQuantities(items); err != nil {
		return nil, err
	}
	return items, nil
}

func inventoryActionFromNode(node *bbcode.BBCodeNode) (InventoryAction, error) {
	tag := node.GetOpeningTag()
	action := InventoryAction{Kind: inventoryActionTags[tag.Name]}
	items, err := parseInventoryItemList(tag.Args["items"])
	if err != nil {
		return action, actionError("[%s] needs items=ID:QUANTITY,… with each item once and a positive quantity", tag.Name)
	}
	action.Items = items
	if action.Kind == InventoryGive {
		to, err := strconv.Atoi(tag.Args["to"])
		if err != nil || to <= 0 {
			return action, actionError("[%s] needs to=CHARACTER_PROFILE_ID", tag.Name)
		}
		action.ToProfileID = to
	}
	return action, nil
}

// ParseInventoryActions returns the inventory actions of a post in document order, skipping [code] blocks.
func ParseInventoryActions(content string) ([]InventoryAction, error) {
	var actions []InventoryAction
	var walk func(node *bbcode.BBCodeNode) error
	walk = func(node *bbcode.BBCodeNode) error {
		for _, child := range node.Children {
			if tag := child.GetOpeningTag(); tag != nil {
				if tag.Name == "code" {
					continue
				}
				if _, ok := inventoryActionTags[tag.Name]; ok {
					action, err := inventoryActionFromNode(child)
					if err != nil {
						return err
					}
					actions = append(actions, action)
				}
			}
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(bbcode.Parse(bbcode.Lex(content))); err != nil {
		return nil, err
	}
	return actions, nil
}

func compileInventoryAction(node *bbcode.BBCodeNode) (*bbcode.HTMLTag, bool) {
	out := bbcode.NewHTMLTag("")
	out.Name = "span"
	if action, err := inventoryActionFromNode(node); err == nil {
		out.Name = "inv-action"
		out.Attrs["data-action"] = action.String()
	}
	return out, true
}

// ─── Applying actions ─────────────────────────────────────────────────────────

// sceneInventoryID returns 0 when the profile has no inventory in the scene.
func sceneInventoryID(q DBExecutor, episodeID, profileID int) (id int, locked bool, err error) {
	err = q.QueryRow("SELECT id, is_locked FROM inventory_scene WHERE episode_id = ? AND character_profile_id = ?",
		episodeID, profileID).Scan(&id, &locked)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("loading inventory of profile %d in scene %d: %w", profileID, episodeID, err)
	}
	return id, locked, nil
}

// inventoryAsOf returns what an inventory held just before a post: its latest earlier post copy, or the starting items.
func inventoryAsOf(tx *sql.Tx, sceneInvID int, postID int64) (map[int]int, error) {
	var prev sql.NullInt64
	if err := tx.QueryRow("SELECT MAX(post_id) FROM inventory_post WHERE scene_inventory_id = ? AND post_id < ?",
		sceneInvID, postID).Scan(&prev); err != nil {
		return nil, fmt.Errorf("finding inventory %d before post %d: %w", sceneInvID, postID, err)
	}
	query, args := "SELECT item_id, quantity FROM inventory_scene_items WHERE scene_inventory_id = ?", []any{sceneInvID}
	if prev.Valid {
		query = "SELECT item_id, quantity FROM inventory_post_items WHERE post_id = ? AND scene_inventory_id = ?"
		args = []any{prev.Int64, sceneInvID}
	}
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("loading inventory %d before post %d: %w", sceneInvID, postID, err)
	}
	defer rows.Close()
	items := map[int]int{}
	for rows.Next() {
		var itemID, qty int
		if err := rows.Scan(&itemID, &qty); err != nil {
			return nil, fmt.Errorf("scanning inventory item: %w", err)
		}
		items[itemID] = qty
	}
	return items, rows.Err()
}

func sortedKeys(m map[int]int) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

func writeInventoryCopy(tx *sql.Tx, postID int64, sceneInvID int, items map[int]int) error {
	if _, err := tx.Exec("INSERT INTO inventory_post (post_id, scene_inventory_id) VALUES (?, ?)", postID, sceneInvID); err != nil {
		return fmt.Errorf("creating inventory copy %d for post %d: %w", sceneInvID, postID, err)
	}
	var values []string
	var args []any
	for _, itemID := range sortedKeys(items) {
		if items[itemID] > 0 {
			values = append(values, "(?, ?, ?, ?)")
			args = append(args, postID, sceneInvID, itemID, items[itemID])
		}
	}
	if len(values) == 0 {
		return nil
	}
	if _, err := tx.Exec("INSERT INTO inventory_post_items (post_id, scene_inventory_id, item_id, quantity) VALUES "+
		strings.Join(values, ", "), args...); err != nil {
		return fmt.Errorf("copying inventory %d for post %d: %w", sceneInvID, postID, err)
	}
	return nil
}

func inPlaceholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func lockSceneInventories(tx *sql.Tx, ids []int) error {
	sort.Ints(ids)
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := tx.Query("SELECT id FROM inventory_scene WHERE id IN ("+inPlaceholders(len(ids))+") ORDER BY id FOR UPDATE", args...)
	if err != nil {
		return fmt.Errorf("locking inventories: %w", err)
	}
	return rows.Close()
}

func itemNames(q DBExecutor, ids []int) (map[int]string, error) {
	names := map[int]string{}
	if len(ids) == 0 {
		return names, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := q.Query("SELECT id, name FROM inventory_items WHERE id IN ("+inPlaceholders(len(ids))+")", args...)
	if err != nil {
		return nil, fmt.Errorf("loading item names: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scanning item name: %w", err)
		}
		names[id] = name
	}
	return names, rows.Err()
}

// resolveRecipients maps each give recipient to its locked inventory in the scene, in one query.
func resolveRecipients(tx *sql.Tx, episodeID, authorProfileID int, actions []InventoryAction) (map[int]int, error) {
	var ids []any
	seen := map[int]bool{}
	for _, a := range actions {
		if a.Kind != InventoryGive || seen[a.ToProfileID] {
			continue
		}
		if a.ToProfileID == authorProfileID {
			return nil, actionError("A character can't give items to themselves")
		}
		seen[a.ToProfileID] = true
		ids = append(ids, a.ToProfileID)
	}
	recipients := map[int]int{}
	if len(ids) == 0 {
		return recipients, nil
	}
	rows, err := tx.Query(`
		SELECT DISTINCT p.id, COALESCE(s.id, 0), COALESCE(s.is_locked, 0)
		FROM (`+sceneProfilesQuery+`) p
		LEFT JOIN inventory_scene s ON s.character_profile_id = p.id AND s.episode_id = ?
		WHERE p.id IN (`+inPlaceholders(len(ids))+`)`,
		append([]any{episodeID, episodeID, episodeID}, ids...)...)
	if err != nil {
		return nil, fmt.Errorf("loading recipients in scene %d: %w", episodeID, err)
	}
	defer rows.Close()
	locked := map[int]bool{}
	for rows.Next() {
		var profileID, invID int
		var isLocked bool
		if err := rows.Scan(&profileID, &invID, &isLocked); err != nil {
			return nil, fmt.Errorf("scanning recipient: %w", err)
		}
		recipients[profileID] = invID
		locked[profileID] = isLocked
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		profileID := id.(int)
		if _, inScene := recipients[profileID]; !inScene {
			return nil, actionError("The recipient of [item-give] is not in this scene")
		}
		if recipients[profileID] == 0 || !locked[profileID] {
			return nil, actionError("The recipient's inventory in this scene is not locked yet")
		}
	}
	return recipients, nil
}

// ApplyPostInventory stores a post's inventory copies inside tx: the author's inventory as of the post with its
// actions applied, plus one copy per give recipient. Posts whose author has no locked inventory in the scene get
// no copy, and actions in them are refused with an *InventoryActionError.
func ApplyPostInventory(tx *sql.Tx, postID, topicID int64, profileID *int, content string) error {
	actions, err := ParseInventoryActions(content)
	if err != nil {
		return err
	}
	var episodeID int
	var status Entities.EpisodeStatus
	var usesInventory bool
	err = tx.QueryRow(`
		SELECT e.id, e.episode_status, EXISTS(SELECT 1 FROM inventory_episode_settings s WHERE s.episode_id = e.id)
		FROM episode_base e WHERE e.topic_id = ?`, topicID).Scan(&episodeID, &status, &usesInventory)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("loading scene of topic %d: %w", topicID, err)
	}
	if err == sql.ErrNoRows || !usesInventory {
		if len(actions) > 0 {
			return actionError("Inventory actions can only be used in scenes that use inventory")
		}
		return nil
	}
	if len(actions) > 0 && status == Entities.PendingEpisode {
		return actionError("Inventory actions can be used once the scene has started")
	}
	if profileID == nil {
		if len(actions) > 0 {
			return actionError("Post as a character to use inventory actions")
		}
		return nil
	}
	authorInv, locked, err := sceneInventoryID(tx, episodeID, *profileID)
	if err != nil {
		return err
	}
	if authorInv == 0 || !locked {
		if len(actions) > 0 {
			return actionError("This character's inventory in the scene is not locked yet")
		}
		return nil
	}

	recipients, err := resolveRecipients(tx, episodeID, *profileID, actions)
	if err != nil {
		return err
	}
	lockIDs := []int{authorInv}
	for _, invID := range recipients {
		lockIDs = append(lockIDs, invID)
	}
	if err := lockSceneInventories(tx, lockIDs); err != nil {
		return err
	}

	held, err := inventoryAsOf(tx, authorInv, postID)
	if err != nil {
		return err
	}
	gifts := map[int]map[int]int{} // recipient inventory -> item -> quantity
	var missing []int
	for _, a := range actions {
		for _, it := range a.Items {
			if held[it.ItemID] < it.Quantity {
				missing = append(missing, it.ItemID)
				continue
			}
			held[it.ItemID] -= it.Quantity
			if a.Kind == InventoryGive {
				invID := recipients[a.ToProfileID]
				if gifts[invID] == nil {
					gifts[invID] = map[int]int{}
				}
				gifts[invID][it.ItemID] += it.Quantity
			}
		}
	}
	if len(missing) > 0 {
		return missingItemsError(tx, missing)
	}

	if err := writeInventoryCopy(tx, postID, authorInv, held); err != nil {
		return err
	}
	for invID, received := range gifts {
		items, err := inventoryAsOf(tx, invID, postID)
		if err != nil {
			return err
		}
		for itemID, qty := range received {
			items[itemID] += qty
		}
		if err := writeInventoryCopy(tx, postID, invID, items); err != nil {
			return err
		}
	}
	return nil
}

func missingItemsError(q DBExecutor, ids []int) error {
	names, err := itemNames(q, ids)
	if err != nil {
		return err
	}
	seen := map[int]bool{}
	var list []string
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if name, ok := names[id]; ok {
			list = append(list, name)
		} else {
			list = append(list, fmt.Sprintf("#%d", id))
		}
	}
	return actionError("Not enough in the inventory: %s", strings.Join(list, ", "))
}

func isLatestPostInTopic(q DBExecutor, postID, topicID int64) (bool, error) {
	var later bool
	if err := q.QueryRow("SELECT EXISTS(SELECT 1 FROM posts WHERE topic_id = ? AND id > ? AND is_deleted IS NOT TRUE)",
		topicID, postID).Scan(&later); err != nil {
		return false, fmt.Errorf("checking for posts after %d: %w", postID, err)
	}
	return !later, nil
}

func sameActions(a, b []InventoryAction) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].String() != b[i].String() {
			return false
		}
	}
	return true
}

// UpdatePostInventory re-applies a post's inventory after an edit. Changing its actions, or the author of a post
// with actions, is only allowed while the post is the latest in its topic (ErrInventoryActionsFrozen otherwise).
func UpdatePostInventory(tx *sql.Tx, postID, topicID int64, oldProfileID, newProfileID *int, oldContent, newContent string) error {
	newActions, err := ParseInventoryActions(newContent)
	if err != nil {
		return err
	}
	oldActions, _ := ParseInventoryActions(oldContent) // validated when it was saved; older posts may hold stray text
	actionsChanged := !sameActions(oldActions, newActions)
	profileChanged := (oldProfileID == nil) != (newProfileID == nil) ||
		(oldProfileID != nil && newProfileID != nil && *oldProfileID != *newProfileID)
	if !actionsChanged && !profileChanged {
		return nil
	}
	latest, err := isLatestPostInTopic(tx, postID, topicID)
	if err != nil {
		return err
	}
	if !latest {
		if actionsChanged || len(newActions) > 0 {
			return ErrInventoryActionsFrozen
		}
		return nil
	}
	if _, err := tx.Exec("DELETE FROM inventory_post WHERE post_id = ?", postID); err != nil {
		return fmt.Errorf("removing inventory copies of post %d: %w", postID, err)
	}
	return ApplyPostInventory(tx, postID, topicID, newProfileID, newContent)
}

// RemovePostInventoryOnDelete undoes a deleted post's actions when it is the latest post of its topic. Copies of
// older posts stay: later inventories were built on them.
func RemovePostInventoryOnDelete(tx *sql.Tx, postID, topicID int64) error {
	latest, err := isLatestPostInTopic(tx, postID, topicID)
	if err != nil || !latest {
		return err
	}
	if _, err := tx.Exec("DELETE FROM inventory_post WHERE post_id = ?", postID); err != nil {
		return fmt.Errorf("removing inventory copies of post %d: %w", postID, err)
	}
	return nil
}

// ListInventoryRecipients returns the profiles of a scene that have a locked inventory, by name.
func ListInventoryRecipients(db *sql.DB, episodeID int) ([]Entities.InventoryRecipient, error) {
	rows, err := db.Query(`
		SELECT DISTINCT p.id, p.name
		FROM (`+sceneProfilesQuery+`) p
		JOIN inventory_scene s ON s.character_profile_id = p.id AND s.episode_id = ? AND s.is_locked = 1
		ORDER BY p.name, p.id`, episodeID, episodeID, episodeID)
	if err != nil {
		return nil, fmt.Errorf("listing inventory recipients of scene %d: %w", episodeID, err)
	}
	defer rows.Close()
	list := []Entities.InventoryRecipient{}
	for rows.Next() {
		var r Entities.InventoryRecipient
		var name sql.NullString
		if err := rows.Scan(&r.ProfileID, &name); err != nil {
			return nil, fmt.Errorf("scanning inventory recipient: %w", err)
		}
		r.Name = name.String
		list = append(list, r)
	}
	return list, rows.Err()
}

// ─── Rendering ────────────────────────────────────────────────────────────────

var invActionRe = regexp.MustCompile(`(?s)<inv-action data-action="(drop|give);(\d+);([\d:,]+)">(.*?)</inv-action>`)

// ResolveInventoryActions turns the <inv-action> placeholders of compiled posts into hoverable spans that list
// what was dropped or given, in the viewer's language. All names are loaded in two queries for every post given.
func ResolveInventoryActions(db *sql.DB, lang string, contents ...*string) error {
	itemIDs, profileIDs := map[int]bool{}, map[int]bool{}
	for _, content := range contents {
		for _, m := range invActionRe.FindAllStringSubmatch(*content, -1) {
			if to, _ := strconv.Atoi(m[2]); to > 0 {
				profileIDs[to] = true
			}
			items, _ := parseInventoryItemList(m[3])
			for _, it := range items {
				itemIDs[it.ItemID] = true
			}
		}
	}
	if len(itemIDs) == 0 {
		return nil
	}
	items, err := itemNames(db, setKeys(itemIDs))
	if err != nil {
		return err
	}
	profiles, err := profileNames(db, setKeys(profileIDs))
	if err != nil {
		return err
	}
	localizer := NewLocalizer(lang)
	for _, content := range contents {
		*content = invActionRe.ReplaceAllStringFunc(*content, func(match string) string {
			return renderInventoryAction(invActionRe.FindStringSubmatch(match), items, profiles, localizer)
		})
	}
	return nil
}

func setKeys(set map[int]bool) []int {
	keys := make([]int, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	return keys
}

func profileNames(db *sql.DB, ids []int) (map[int]string, error) {
	names := map[int]string{}
	if len(ids) == 0 {
		return names, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := db.Query(`
		SELECT cpb.id, IF(cpb.is_mask, cpb.mask_name, cb.name)
		FROM character_profile_base cpb LEFT JOIN character_base cb ON cb.id = cpb.character_id
		WHERE cpb.id IN (`+inPlaceholders(len(ids))+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("loading character names: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var name sql.NullString
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scanning character name: %w", err)
		}
		names[id] = name.String
	}
	return names, rows.Err()
}

func renderInventoryAction(m []string, items, profiles map[int]string, localizer *i18n.Localizer) string {
	kind, text := m[1], m[4]
	list, _ := parseInventoryItemList(m[3])
	entries := make([]string, len(list))
	for i, it := range list {
		name, ok := items[it.ItemID]
		if !ok {
			name = fmt.Sprintf("#%d", it.ItemID)
		}
		entries[i] = fmt.Sprintf("%s ×%d", name, it.Quantity)
	}
	label := T(localizer, "inventory.action.dropped")
	if kind == string(InventoryGive) {
		to, _ := strconv.Atoi(m[2])
		label = TData(localizer, "inventory.action.given_to", map[string]interface{}{"Name": profiles[to]})
	}
	// The hover text lives in an attribute shown by CSS, so a forum stylesheet without the rule shows plain text.
	return fmt.Sprintf(`<span class="inv-action inv-action--%s" tabindex="0" data-tip="%s">%s</span>`,
		kind, html.EscapeString(label+": "+strings.Join(entries, ", ")), text)
}

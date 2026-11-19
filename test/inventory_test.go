package test

import (
	"cuento-backend/src/Controllers"
	"cuento-backend/src/Entities"
	"cuento-backend/src/Services"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// invScene is an arc with inventory enabled and one scene that uses it.
// The player and the other player each have a character in the scene; the mask belongs to maskOwner.
type invScene struct {
	arcID, episodeID, topicID                int
	gm, player, other, maskOwner             int
	playerProfile, otherProfile, maskProfile int
	sword, coin                              int
}

func newInvScene(t *testing.T, f *fixture, subforumID int, status Entities.EpisodeStatus) *invScene {
	t.Helper()
	s := &invScene{
		gm:        f.addUser(t, "gm", roleUser),
		player:    f.member,
		other:     f.addUser(t, "other", roleUser),
		maskOwner: f.addUser(t, "mask-owner", roleUser),
	}
	s.arcID = f.insert(t, "INSERT INTO arcs (title, is_public, status) VALUES ('Campaign', 1, 1)")
	f.exec(t, "INSERT INTO arc_game_masters (arc_id, user_id) VALUES (?, ?)", s.arcID, s.gm)
	f.exec(t, "INSERT INTO inventory_arc_settings (arc_id, enabled_at) VALUES (?, NOW())", s.arcID)

	s.topicID = f.addTopic(t, subforumID, s.gm, Entities.EpisodeTopic, "Heist")
	s.episodeID = f.insert(t, "INSERT INTO episode_base (topic_id, name, episode_status) VALUES (?, 'Heist', ?)", s.topicID, status)
	f.exec(t, "INSERT INTO arc_episodes (arc_id, episode_id) VALUES (?, ?)", s.arcID, s.episodeID)
	f.exec(t, "INSERT INTO inventory_episode_settings (episode_id) VALUES (?)", s.episodeID)

	s.playerProfile = f.addSceneCharacter(t, s.episodeID, s.player, "Alice")
	s.otherProfile = f.addSceneCharacter(t, s.episodeID, s.other, "Bob")
	s.maskProfile = f.insert(t, "INSERT INTO character_profile_base (user_id, is_mask, mask_name) VALUES (?, 1, 'The Stranger')", s.maskOwner)
	f.exec(t, "INSERT INTO episode_mask (episode_id, mask_id) VALUES (?, ?)", s.episodeID, s.maskProfile)

	s.sword = f.insert(t, "INSERT INTO inventory_items (arc_id, name) VALUES (?, 'Sword')", s.arcID)
	s.coin = f.insert(t, "INSERT INTO inventory_items (arc_id, name) VALUES (?, 'Gold coin')", s.arcID)
	return s
}

// addSceneCharacter adds a character of userID to the scene and returns its main profile id.
func (f *fixture) addSceneCharacter(t *testing.T, episodeID, userID int, name string) int {
	t.Helper()
	characterID, _ := f.addCharacter(t, f.subA, userID, name)
	f.exec(t, "INSERT INTO episode_character (episode_id, character_id) VALUES (?, ?)", episodeID, characterID)
	return f.count(t, "SELECT id FROM character_profile_base WHERE character_id = ?", characterID)
}

// fillAndLock gives a profile a starting inventory and locks it, straight in the database.
func (s *invScene) fillAndLock(t *testing.T, f *fixture, profileID int, items map[int]int) int {
	t.Helper()
	id := f.insert(t, "INSERT INTO inventory_scene (episode_id, character_profile_id, is_locked, locked_at) VALUES (?, ?, 1, NOW())", s.episodeID, profileID)
	for itemID, qty := range items {
		f.exec(t, "INSERT INTO inventory_scene_items (scene_inventory_id, item_id, quantity) VALUES (?, ?, ?)", id, itemID, qty)
	}
	return id
}

func (s *invScene) addPost(t *testing.T, f *fixture, userID, profileID int) int {
	t.Helper()
	return f.insert(t, "INSERT INTO posts (topic_id, author_user_id, content, use_character_profile, character_profile_id) VALUES (?, ?, 'post', 1, ?)",
		s.topicID, userID, profileID)
}

func decode[T any](t *testing.T, body []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decoding %s: %v", body, err)
	}
	return v
}

func quantities(entries []Entities.InventoryEntry) map[string]int {
	m := map[string]int{}
	for _, e := range entries {
		m[e.Name] = e.Quantity
	}
	return m
}

func TestValidateInventoryQuantities(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		items []Services.InventoryQuantity
		ok    bool
	}{
		{"empty", nil, true},
		{"valid", []Services.InventoryQuantity{{ItemID: 1, Quantity: 20}, {ItemID: 2, Quantity: 1}}, true},
		{"zero quantity", []Services.InventoryQuantity{{ItemID: 1, Quantity: 0}}, false},
		{"negative quantity", []Services.InventoryQuantity{{ItemID: 1, Quantity: -3}}, false},
		{"duplicate item", []Services.InventoryQuantity{{ItemID: 1, Quantity: 1}, {ItemID: 1, Quantity: 2}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Services.ValidateInventoryQuantities(tt.items)
			if (err == nil) != tt.ok {
				t.Errorf("expected ok=%v, got %v", tt.ok, err)
			}
		})
	}
}

// Every protected inventory endpoint: unauthenticated, wrong global role, logged in but not a GM, GM.
func TestInventory_EndpointAccess(t *testing.T) {
	type request struct {
		method, path string
		body         any
	}
	endpoints := []struct {
		name string
		req  func(s *invScene) request
		want int
	}{
		{"arc enable", func(s *invScene) request {
			return request{"POST", fmt.Sprintf("/arc/%d/inventory/enable", s.arcID), nil}
		}, http.StatusOK},
		// the scene already uses inventory, so even the GM gets 409 once past the permission checks
		{"arc disable", func(s *invScene) request {
			return request{"POST", fmt.Sprintf("/arc/%d/inventory/disable", s.arcID), nil}
		}, http.StatusConflict},
		{"item create", func(s *invScene) request {
			return request{"POST", "/inventory/item/create", map[string]any{"arc_id": s.arcID, "name": "Rope"}}
		}, http.StatusCreated},
		{"item update", func(s *invScene) request {
			return request{"POST", fmt.Sprintf("/inventory/item/update/%d", s.sword), map[string]any{"name": "Blade"}}
		}, http.StatusOK},
		{"item delete", func(s *invScene) request {
			return request{"DELETE", fmt.Sprintf("/inventory/item/delete/%d", s.coin), nil}
		}, http.StatusOK},
		{"scene settings", func(s *invScene) request {
			return request{"POST", fmt.Sprintf("/episode/%d/inventory/settings", s.episodeID), map[string]any{"use_inventory": true}}
		}, http.StatusOK},
		{"scene inventories", func(s *invScene) request {
			return request{"GET", fmt.Sprintf("/episode/%d/inventories", s.episodeID), nil}
		}, http.StatusOK},
		{"scene update", func(s *invScene) request {
			return request{"POST", fmt.Sprintf("/episode/%d/inventory/%d/update", s.episodeID, s.playerProfile), map[string]any{"items": []any{}}}
		}, http.StatusOK},
		{"scene lock", func(s *invScene) request {
			return request{"POST", fmt.Sprintf("/episode/%d/inventory/%d/lock", s.episodeID, s.playerProfile), nil}
		}, http.StatusOK},
		{"scene start", func(s *invScene) request { return request{"POST", fmt.Sprintf("/episode/%d/start", s.episodeID), nil} }, http.StatusConflict},
	}
	callers := []struct {
		name    string
		caller  func(f *fixture, s *invScene) int
		success bool
		want    int
	}{
		{"unauthenticated", func(*fixture, *invScene) int { return 0 }, false, http.StatusUnauthorized},
		{"guest role", func(f *fixture, _ *invScene) int { return f.guestUser }, false, http.StatusForbidden},
		{"player, not a GM", func(_ *fixture, s *invScene) int { return s.player }, false, http.StatusForbidden},
		{"GM", func(_ *fixture, s *invScene) int { return s.gm }, true, 0},
	}
	for _, ep := range endpoints {
		for _, c := range callers {
			t.Run(ep.name+"/"+c.name, func(t *testing.T) {
				f := newFixture(t)
				s := newInvScene(t, f, f.subA, Entities.PendingEpisode)
				r := ep.req(s)
				want := c.want
				if c.success {
					want = ep.want
				}
				expectStatus(t, f.do(t, r.method, r.path, c.caller(f, s), r.body), want)
			})
		}
	}
}

// Scene endpoints touch topics, so both the global role and the subforum ACL must pass.
func TestInventory_SceneSubforumAccess(t *testing.T) {
	paths := []struct {
		name, method string
		path         func(s *invScene) string
		want         int
	}{
		{"inventories", "GET", func(s *invScene) string { return fmt.Sprintf("/episode/%d/inventories", s.episodeID) }, http.StatusOK},
		{"lock", "POST", func(s *invScene) string {
			return fmt.Sprintf("/episode/%d/inventory/%d/lock", s.episodeID, s.playerProfile)
		}, http.StatusOK},
		{"start", "POST", func(s *invScene) string { return fmt.Sprintf("/episode/%d/start", s.episodeID) }, http.StatusConflict},
		{"current", "GET", func(s *invScene) string { return fmt.Sprintf("/episode/%d/inventory/current", s.episodeID) }, http.StatusOK},
		{"update", "POST", func(s *invScene) string {
			return fmt.Sprintf("/episode/%d/inventory/%d/update", s.episodeID, s.playerProfile)
		}, http.StatusOK},
		{"settings", "POST", func(s *invScene) string { return fmt.Sprintf("/episode/%d/inventory/settings", s.episodeID) }, http.StatusOK},
	}
	cases := []struct {
		name     string
		subforum func(f *fixture) int
		caller   func(f *fixture, s *invScene) int
		success  bool
	}{
		// gm holds only the user role: permissions in subA, and none in the private subforum (not listed there)
		{"GM role but no subforum permission", func(f *fixture) int { return f.subPrivate }, func(_ *fixture, s *invScene) int { return s.gm }, false},
		{"GM with permission for a different subforum", func(f *fixture) int { return f.subB }, func(_ *fixture, s *invScene) int { return s.gm }, false},
		// guestUser has every subA permission, but the guest role lacks the endpoint permission
		{"subforum permission but insufficient role", func(f *fixture) int { return f.subA }, func(f *fixture, _ *invScene) int { return f.guestUser }, false},
		{"role and subforum permission", func(f *fixture) int { return f.subA }, func(_ *fixture, s *invScene) int { return s.gm }, true},
		// admin holds subB permissions and is a GM of every arc through /arc/update/:id
		{"admin in subB", func(f *fixture) int { return f.subB }, func(f *fixture, _ *invScene) int { return f.admin }, true},
	}
	for _, p := range paths {
		for _, tc := range cases {
			t.Run(p.name+"/"+tc.name, func(t *testing.T) {
				f := newFixture(t)
				s := newInvScene(t, f, tc.subforum(f), Entities.PendingEpisode)
				want := http.StatusForbidden
				if tc.success {
					want = p.want
				}
				body := map[string]any{"items": []any{}, "use_inventory": true}
				expectStatus(t, f.do(t, p.method, p.path(s), tc.caller(f, s), body), want)
			})
		}
	}
}

func TestCurrentSceneInventories_Access(t *testing.T) {
	tests := []struct {
		name   string
		caller func(f *fixture, s *invScene) int
		want   int
	}{
		{"unauthenticated", func(*fixture, *invScene) int { return 0 }, http.StatusUnauthorized},
		{"guest role with subforum permission", func(f *fixture, _ *invScene) int { return f.guestUser }, http.StatusForbidden},
		{"player", func(_ *fixture, s *invScene) int { return s.player }, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)
			expectStatus(t, f.do(t, "GET", fmt.Sprintf("/episode/%d/inventory/current", s.episodeID), tt.caller(f, s), nil), tt.want)
		})
	}
}

func TestPostInventory_Access(t *testing.T) {
	tests := []struct {
		name     string
		subforum func(f *fixture) int
		caller   func(f *fixture, s *invScene) int
		want     int
	}{
		{"unauthenticated", func(f *fixture) int { return f.subA }, func(*fixture, *invScene) int { return 0 }, http.StatusUnauthorized},
		{"guest role with subforum permission", func(f *fixture) int { return f.subA }, func(f *fixture, _ *invScene) int { return f.guestUser }, http.StatusForbidden},
		{"the post's player", func(f *fixture) int { return f.subA }, func(_ *fixture, s *invScene) int { return s.player }, http.StatusOK},
		{"another player in the scene", func(f *fixture) int { return f.subA }, func(_ *fixture, s *invScene) int { return s.other }, http.StatusForbidden},
		{"GM", func(f *fixture) int { return f.subA }, func(_ *fixture, s *invScene) int { return s.gm }, http.StatusOK},
		{"player without subforum permission", func(f *fixture) int { return f.subB }, func(_ *fixture, s *invScene) int { return s.player }, http.StatusForbidden},
		{"GM with permission for a different subforum", func(f *fixture) int { return f.subB }, func(_ *fixture, s *invScene) int { return s.gm }, http.StatusForbidden},
		{"admin GM with subforum permission", func(f *fixture) int { return f.subB }, func(f *fixture, _ *invScene) int { return f.admin }, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			s := newInvScene(t, f, tt.subforum(f), Entities.ActiveEpisode)
			s.fillAndLock(t, f, s.playerProfile, map[int]int{s.coin: 20})
			postID := s.addPost(t, f, s.player, s.playerProfile)
			if err := Services.SnapshotPostInventory(f.db, int64(postID)); err != nil {
				t.Fatalf("snapshot: %v", err)
			}

			w := f.do(t, "GET", fmt.Sprintf("/post/%d/inventory", postID), tt.caller(f, s), nil)
			expectStatus(t, w, tt.want)
			if tt.want == http.StatusOK {
				got := decode[Controllers.PostInventoryResponse](t, w.Body.Bytes())
				if got.Name != "Alice" || quantities(got.Items)["Gold coin"] != 20 {
					t.Errorf("unexpected inventory: %s", w.Body.String())
				}
			}
		})
	}
}

func TestPostInventory_EmptyAndMissing(t *testing.T) {
	f := newFixture(t)
	s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)

	// Posted before the inventory was locked: no copy, so the inventory is empty.
	early := s.addPost(t, f, s.player, s.playerProfile)
	w := f.do(t, "GET", fmt.Sprintf("/post/%d/inventory", early), s.player, nil)
	expectStatus(t, w, http.StatusOK)
	if got := decode[Controllers.PostInventoryResponse](t, w.Body.Bytes()); len(got.Items) != 0 {
		t.Errorf("expected an empty inventory, got %s", w.Body.String())
	}

	// An out-of-character post has no character inventory at all.
	ooc := f.insert(t, "INSERT INTO posts (topic_id, author_user_id, content) VALUES (?, ?, 'ooc')", s.topicID, s.player)
	expectStatus(t, f.do(t, "GET", fmt.Sprintf("/post/%d/inventory", ooc), s.player, nil), http.StatusNotFound)

	// A scene that does not use inventory refuses.
	f.exec(t, "DELETE FROM inventory_episode_settings WHERE episode_id = ?", s.episodeID)
	expectStatus(t, f.do(t, "GET", fmt.Sprintf("/post/%d/inventory", early), s.player, nil), http.StatusConflict)
}

func TestArcInventory_EnableDisable(t *testing.T) {
	f := newFixture(t)
	s := newInvScene(t, f, f.subA, Entities.PendingEpisode)
	disable := fmt.Sprintf("/arc/%d/inventory/disable", s.arcID)

	expectStatus(t, f.do(t, "POST", disable, s.gm, nil), http.StatusConflict)

	f.exec(t, "DELETE FROM inventory_episode_settings WHERE episode_id = ?", s.episodeID)
	expectStatus(t, f.do(t, "POST", disable, s.gm, nil), http.StatusOK)
	if n := f.count(t, "SELECT COUNT(*) FROM inventory_arc_settings WHERE arc_id = ?", s.arcID); n != 0 {
		t.Errorf("expected inventory to be off, got %d settings rows", n)
	}

	// Items can only be added while inventory is on.
	w := f.do(t, "POST", "/inventory/item/create", s.gm, map[string]any{"arc_id": s.arcID, "name": "Rope"})
	expectStatus(t, w, http.StatusConflict)

	expectStatus(t, f.do(t, "POST", fmt.Sprintf("/arc/%d/inventory/enable", s.arcID), s.gm, nil), http.StatusOK)
	expectStatus(t, f.do(t, "POST", fmt.Sprintf("/arc/%d/inventory/enable", s.arcID), s.gm, nil), http.StatusOK)
}

func TestInventoryItem_HiddenVisibility(t *testing.T) {
	f := newFixture(t)
	s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)
	f.exec(t, "UPDATE inventory_items SET is_hidden = 1 WHERE id = ?", s.sword)

	tests := []struct {
		name      string
		caller    func() int
		wantNames string
		wantItem  int
	}{
		{"guest", func() int { return 0 }, "Gold coin", http.StatusNotFound},
		{"player", func() int { return s.player }, "Gold coin", http.StatusNotFound},
		{"GM", func() int { return s.gm }, "Sword,Gold coin", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := f.do(t, "GET", fmt.Sprintf("/arc/%d/inventory/items", s.arcID), tt.caller(), nil)
			expectStatus(t, w, http.StatusOK)
			got := decode[Controllers.ArcInventoryResponse](t, w.Body.Bytes())
			var names []string
			for _, it := range got.Items {
				names = append(names, it.Name)
			}
			if strings.Join(names, ",") != tt.wantNames {
				t.Errorf("expected items %q, got %q", tt.wantNames, strings.Join(names, ","))
			}
			expectStatus(t, f.do(t, "GET", fmt.Sprintf("/inventory/item/%d", s.sword), tt.caller(), nil), tt.wantItem)
		})
	}
}

func TestInventoryItem_Validation(t *testing.T) {
	tests := []struct {
		name string
		body map[string]any
	}{
		{"blank name", map[string]any{"name": "   "}},
		{"name too long", map[string]any{"name": strings.Repeat("a", 256)}},
		{"icon too long", map[string]any{"name": "Rope", "icon": strings.Repeat("a", 513)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)
			tt.body["arc_id"] = s.arcID
			expectStatus(t, f.do(t, "POST", "/inventory/item/create", s.gm, tt.body), http.StatusBadRequest)
		})
	}
}

func TestInventoryItem_DeleteUsedInLockedInventory(t *testing.T) {
	f := newFixture(t)
	s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)
	s.fillAndLock(t, f, s.playerProfile, map[int]int{s.sword: 1})

	expectStatus(t, f.do(t, "DELETE", fmt.Sprintf("/inventory/item/delete/%d", s.sword), s.gm, nil), http.StatusConflict)
	expectStatus(t, f.do(t, "DELETE", fmt.Sprintf("/inventory/item/delete/%d", s.coin), s.gm, nil), http.StatusOK)
	if n := f.count(t, "SELECT COUNT(*) FROM inventory_items WHERE arc_id = ?", s.arcID); n != 1 {
		t.Errorf("expected 1 item left, got %d", n)
	}
}

func TestSceneInventory_SaveAndLock(t *testing.T) {
	f := newFixture(t)
	s := newInvScene(t, f, f.subA, Entities.PendingEpisode)
	update := func(profileID int) string {
		return fmt.Sprintf("/episode/%d/inventory/%d/update", s.episodeID, profileID)
	}
	items := map[string]any{"items": []map[string]int{{"item_id": s.coin, "quantity": 20}, {"item_id": s.sword, "quantity": 1}}}

	expectStatus(t, f.do(t, "POST", update(s.playerProfile), s.gm, items), http.StatusOK)
	expectStatus(t, f.do(t, "POST", fmt.Sprintf("/episode/%d/inventory/%d/lock", s.episodeID, s.playerProfile), s.gm, nil), http.StatusOK)

	w := f.do(t, "GET", fmt.Sprintf("/episode/%d/inventories", s.episodeID), s.gm, nil)
	expectStatus(t, w, http.StatusOK)
	got := decode[Controllers.SceneInventoriesResponse](t, w.Body.Bytes())
	if len(got.Inventories) != 3 {
		t.Fatalf("expected 3 profiles (2 characters and a mask), got %s", w.Body.String())
	}
	for _, inv := range got.Inventories {
		switch inv.ProfileID {
		case s.playerProfile:
			if !inv.IsLocked || inv.LockedAtLocalized == "" || quantities(inv.Items)["Gold coin"] != 20 {
				t.Errorf("player inventory not saved and locked: %+v", inv)
			}
		case s.maskProfile:
			if !inv.IsMask || inv.Name != "The Stranger" || inv.IsLocked {
				t.Errorf("unexpected mask inventory: %+v", inv)
			}
		}
	}
	if strings.Contains(w.Body.String(), `"locked_at":"20`) {
		t.Errorf("lock time must be sent localized, not raw: %s", w.Body.String())
	}

	// Locked: no more edits. The other profiles stay editable.
	expectStatus(t, f.do(t, "POST", update(s.playerProfile), s.gm, items), http.StatusConflict)
	expectStatus(t, f.do(t, "POST", update(s.otherProfile), s.gm, items), http.StatusOK)
	expectStatus(t, f.do(t, "POST", update(s.maskProfile), s.gm, items), http.StatusOK)
}

func TestSceneInventory_InvalidItems(t *testing.T) {
	tests := []struct {
		name    string
		profile func(s *invScene) int
		items   func(f *fixture, s *invScene) []map[string]int
	}{
		{"item of another arc", func(s *invScene) int { return s.playerProfile }, func(f *fixture, s *invScene) []map[string]int {
			otherArc := f.insert(t, "INSERT INTO arcs (title, is_public) VALUES ('Other', 1)")
			foreign := f.insert(t, "INSERT INTO inventory_items (arc_id, name) VALUES (?, 'Foreign')", otherArc)
			return []map[string]int{{"item_id": foreign, "quantity": 1}}
		}},
		{"zero quantity", func(s *invScene) int { return s.playerProfile }, func(_ *fixture, s *invScene) []map[string]int {
			return []map[string]int{{"item_id": s.coin, "quantity": 0}}
		}},
		{"duplicate item", func(s *invScene) int { return s.playerProfile }, func(_ *fixture, s *invScene) []map[string]int {
			return []map[string]int{{"item_id": s.coin, "quantity": 1}, {"item_id": s.coin, "quantity": 2}}
		}},
		{"profile not in the scene", func(s *invScene) int { return 999999 }, func(_ *fixture, s *invScene) []map[string]int {
			return []map[string]int{{"item_id": s.coin, "quantity": 1}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			s := newInvScene(t, f, f.subA, Entities.PendingEpisode)
			path := fmt.Sprintf("/episode/%d/inventory/%d/update", s.episodeID, tt.profile(s))
			expectStatus(t, f.do(t, "POST", path, s.gm, map[string]any{"items": tt.items(f, s)}), http.StatusBadRequest)
			if n := f.count(t, "SELECT COUNT(*) FROM inventory_scene_items"); n != 0 {
				t.Errorf("expected nothing saved, got %d rows", n)
			}
		})
	}
}

func TestStartScene(t *testing.T) {
	f := newFixture(t)
	s := newInvScene(t, f, f.subA, Entities.PendingEpisode)
	start := fmt.Sprintf("/episode/%d/start", s.episodeID)
	s.fillAndLock(t, f, s.playerProfile, map[int]int{s.coin: 20})

	w := f.do(t, "POST", start, s.gm, nil)
	expectStatus(t, w, http.StatusConflict)
	unlocked := decode[struct{ Unlocked []string }](t, w.Body.Bytes()).Unlocked
	if strings.Join(unlocked, ",") != "Bob,The Stranger" {
		t.Errorf("expected Bob and The Stranger to be reported unlocked, got %v", unlocked)
	}

	// An empty inventory counts once it is locked.
	s.fillAndLock(t, f, s.otherProfile, nil)
	s.fillAndLock(t, f, s.maskProfile, nil)
	expectStatus(t, f.do(t, "POST", start, s.gm, nil), http.StatusOK)

	if st := f.count(t, "SELECT episode_status FROM episode_base WHERE id = ?", s.episodeID); st != int(Entities.ActiveEpisode) {
		t.Errorf("expected the scene to be active, got status %d", st)
	}
	if n := f.count(t, "SELECT COUNT(*) FROM topic_activity_log WHERE topic_id = ? AND event = 'episode_status_changed' AND old_state = 3 AND new_state = 0", s.topicID); n != 1 {
		t.Errorf("expected the start to be logged, got %d rows", n)
	}
	expectStatus(t, f.do(t, "POST", start, s.gm, nil), http.StatusConflict)
}

func TestPendingScene_StatusEndpointsRefuse(t *testing.T) {
	f := newFixture(t)
	s := newInvScene(t, f, f.subA, Entities.PendingEpisode)
	w := f.do(t, "POST", fmt.Sprintf("/episode/%d/status", s.episodeID), s.player, map[string]any{"status": int(Entities.ActiveEpisode)})
	expectStatus(t, w, http.StatusConflict)
	if st := f.count(t, "SELECT episode_status FROM episode_base WHERE id = ?", s.episodeID); st != int(Entities.PendingEpisode) {
		t.Errorf("expected the scene to stay pending, got %d", st)
	}
}

func TestCreatePost_PendingScene(t *testing.T) {
	tests := []struct {
		name   string
		status Entities.EpisodeStatus
		player bool
		want   int
	}{
		{"player in a pending scene", Entities.PendingEpisode, true, http.StatusForbidden},
		{"GM in a pending scene", Entities.PendingEpisode, false, http.StatusCreated},
		{"player in a started scene", Entities.ActiveEpisode, true, http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			s := newInvScene(t, f, f.subA, tt.status)
			req := Controllers.CreatePostRequest{TopicID: s.topicID, Content: "Hello"}
			author := s.gm
			if tt.player {
				author = s.player
				req.UseCharacterProfile = true
				req.CharacterProfileID = &s.playerProfile
			}
			expectStatus(t, f.do(t, "POST", "/post/create", author, req), tt.want)
		})
	}
}

func TestCreateEpisode_UseInventory(t *testing.T) {
	tests := []struct {
		name        string
		creator     func(f *fixture, s *invScene) int
		arcEnabled  bool
		withArc     bool
		want        int
		wantPending bool
	}{
		{"GM in an arc with inventory", func(_ *fixture, s *invScene) int { return s.gm }, true, true, http.StatusCreated, true},
		{"player who is not a GM", func(_ *fixture, s *invScene) int { return s.player }, true, true, http.StatusForbidden, false},
		{"arc without inventory", func(_ *fixture, s *invScene) int { return s.gm }, false, true, http.StatusConflict, false},
		{"no arc", func(_ *fixture, s *invScene) int { return s.gm }, true, false, http.StatusBadRequest, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)
			if !tt.arcEnabled {
				f.exec(t, "DELETE FROM inventory_arc_settings WHERE arc_id = ?", s.arcID)
			}
			body := map[string]any{"subforum_id": f.subA, "name": "New scene", "use_inventory": true}
			if tt.withArc {
				body["arc_id"] = s.arcID
			}
			w := f.do(t, "POST", "/episode/create", tt.creator(f, s), body)
			expectStatus(t, w, tt.want)

			pending := f.count(t, `SELECT COUNT(*) FROM episode_base e JOIN inventory_episode_settings ies ON ies.episode_id = e.id
				WHERE e.name = 'New scene' AND e.episode_status = ?`, Entities.PendingEpisode)
			if (pending == 1) != tt.wantPending {
				t.Errorf("expected pending scene with inventory=%v, got %d", tt.wantPending, pending)
			}
		})
	}
}

func TestSetSceneInventory(t *testing.T) {
	tests := []struct {
		name   string
		status Entities.EpisodeStatus
		locked bool
		on     bool
		want   int
	}{
		{"off while pending", Entities.PendingEpisode, false, false, http.StatusConflict},
		{"off in a started scene with a locked inventory", Entities.ActiveEpisode, true, false, http.StatusConflict},
		{"off in a started scene without locks", Entities.ActiveEpisode, false, false, http.StatusOK},
		{"on", Entities.ActiveEpisode, false, true, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			s := newInvScene(t, f, f.subA, tt.status)
			if tt.locked {
				s.fillAndLock(t, f, s.playerProfile, nil)
			}
			w := f.do(t, "POST", fmt.Sprintf("/episode/%d/inventory/settings", s.episodeID), s.gm, map[string]any{"use_inventory": tt.on})
			expectStatus(t, w, tt.want)
		})
	}

	t.Run("on when the arc has inventory off", func(t *testing.T) {
		f := newFixture(t)
		s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)
		f.exec(t, "DELETE FROM inventory_episode_settings WHERE episode_id = ?", s.episodeID)
		f.exec(t, "DELETE FROM inventory_arc_settings WHERE arc_id = ?", s.arcID)
		w := f.do(t, "POST", fmt.Sprintf("/episode/%d/inventory/settings", s.episodeID), s.gm, map[string]any{"use_inventory": true})
		expectStatus(t, w, http.StatusConflict)
	})
}

func TestSnapshotPostInventory(t *testing.T) {
	f := newFixture(t)
	s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)
	snapshot := func(postID int) {
		t.Helper()
		if err := Services.SnapshotPostInventory(f.db, int64(postID)); err != nil {
			t.Fatalf("snapshot of post %d: %v", postID, err)
		}
	}
	itemsOf := func(postID int) map[string]int {
		t.Helper()
		items, err := Services.GetPostInventoryItems(f.db, postID)
		if err != nil {
			t.Fatalf("loading post %d inventory: %v", postID, err)
		}
		return quantities(items)
	}

	before := s.addPost(t, f, s.player, s.playerProfile)
	snapshot(before)
	if n := f.count(t, "SELECT COUNT(*) FROM inventory_post WHERE post_id = ?", before); n != 0 {
		t.Errorf("a post before the lock must get no copy, got %d", n)
	}

	s.fillAndLock(t, f, s.playerProfile, map[int]int{s.coin: 20, s.sword: 1})
	first := s.addPost(t, f, s.player, s.playerProfile)
	snapshot(first)
	snapshot(first) // safe to run twice
	if got := itemsOf(first); got["Gold coin"] != 20 || got["Sword"] != 1 || len(got) != 2 {
		t.Errorf("first post should copy the starting inventory, got %v", got)
	}

	// Simulate spending in the first post: the next post continues from it, not from the start.
	f.exec(t, "UPDATE inventory_post_items SET quantity = 5 WHERE post_id = ? AND item_id = ?", first, s.coin)
	second := s.addPost(t, f, s.player, s.playerProfile)
	snapshot(second)
	if got := itemsOf(second); got["Gold coin"] != 5 {
		t.Errorf("second post should continue from the first post's copy, got %v", got)
	}

	ooc := f.insert(t, "INSERT INTO posts (topic_id, author_user_id, content, use_character_profile, character_profile_id) VALUES (?, ?, 'ooc', 0, ?)",
		s.topicID, s.player, s.playerProfile)
	snapshot(ooc)
	if n := f.count(t, "SELECT COUNT(*) FROM inventory_post WHERE post_id = ?", ooc); n != 0 {
		t.Errorf("an out-of-character post must get no copy, got %d", n)
	}
}

func TestCurrentSceneInventories(t *testing.T) {
	f := newFixture(t)
	s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)
	s.fillAndLock(t, f, s.playerProfile, map[int]int{s.coin: 20})
	s.fillAndLock(t, f, s.maskProfile, map[int]int{s.sword: 1})
	post := s.addPost(t, f, s.player, s.playerProfile)
	if err := Services.SnapshotPostInventory(f.db, int64(post)); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	f.exec(t, "UPDATE inventory_post_items SET quantity = 7 WHERE post_id = ?", post)

	tests := []struct {
		name      string
		caller    int
		wantNames string
	}{
		{"player sees only their character", s.player, "Alice"},
		{"mask owner sees the mask", s.maskOwner, "The Stranger"},
		{"GM sees everyone", s.gm, "Alice,Bob,The Stranger"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := f.do(t, "GET", fmt.Sprintf("/episode/%d/inventory/current", s.episodeID), tt.caller, nil)
			expectStatus(t, w, http.StatusOK)
			list := decode[[]Entities.SceneInventory](t, w.Body.Bytes())
			var names []string
			for _, inv := range list {
				names = append(names, inv.Name)
				if inv.ProfileID == s.playerProfile && quantities(inv.Items)["Gold coin"] != 7 {
					t.Errorf("current inventory should be the latest post copy, got %+v", inv.Items)
				}
			}
			if strings.Join(names, ",") != tt.wantNames {
				t.Errorf("expected %q, got %q", tt.wantNames, strings.Join(names, ","))
			}
		})
	}
}

func TestSceneInventoryContext_MissingScene(t *testing.T) {
	f := newFixture(t)
	_, err := Services.GetSceneInventoryContext(f.db, 999999)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected no rows for a missing scene, got %v", err)
	}
}

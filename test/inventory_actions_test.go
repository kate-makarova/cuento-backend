package test

import (
	"cuento-backend/src/Controllers"
	"cuento-backend/src/Entities"
	"cuento-backend/src/Services"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestParseInventoryActions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    []string // canonical action strings
		wantErr bool
	}{
		{"no actions", "Just walking.", nil, false},
		{"drop of several items", "He [item-drop items=12:1,15:3]threw out the metal[/item-drop].", []string{"drop;0;12:1,15:3"}, false},
		{"give", "[item-give to=345 items=7:2]rations[/item-give]", []string{"give;345;7:2"}, false},
		{"actions in order, inside other tags", "[b][item-give to=9 items=1:1]a[/item-give][/b] [quote][item-drop items=2:5]b[/item-drop][/quote]",
			[]string{"give;9;1:1", "drop;0;2:5"}, false},
		{"code blocks are skipped", "[code][item-drop items=x]shown as text[/item-drop][/code]", nil, false},
		{"missing items", "[item-drop]x[/item-drop]", nil, true},
		{"zero quantity", "[item-drop items=12:0]x[/item-drop]", nil, true},
		{"duplicate item", "[item-drop items=12:1,12:2]x[/item-drop]", nil, true},
		{"malformed pair", "[item-drop items=12]x[/item-drop]", nil, true},
		{"give without recipient", "[item-give items=12:1]x[/item-give]", nil, true},
		{"give to a non-number", "[item-give to=bob items=12:1]x[/item-give]", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actions, err := Services.ParseInventoryActions(tt.content)
			if tt.wantErr {
				var actionErr *Services.InventoryActionError
				if !errors.As(err, &actionErr) {
					t.Fatalf("expected an InventoryActionError, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var got []string
			for _, a := range actions {
				got = append(got, a.String())
			}
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

// publish creates a post through the API as the player's character.
func (s *invScene) publish(t *testing.T, f *fixture, userID, profileID int, content string) (int, int) {
	t.Helper()
	w := f.do(t, "POST", "/post/create", userID, map[string]any{
		"topic_id": s.topicID, "content": content, "use_character_profile": true, "character_profile_id": profileID,
	})
	if w.Code != http.StatusCreated {
		return 0, w.Code
	}
	return int(decode[map[string]any](t, w.Body.Bytes())["post_id"].(float64)), w.Code
}

func currentOf(t *testing.T, f *fixture, s *invScene, profileID int) map[string]int {
	t.Helper()
	list, err := Services.GetCurrentSceneInventories(f.db, s.episodeID)
	if err != nil {
		t.Fatalf("loading current inventories: %v", err)
	}
	for _, inv := range list {
		if inv.ProfileID == profileID {
			return quantities(inv.Items)
		}
	}
	t.Fatalf("profile %d has no inventory", profileID)
	return nil
}

func TestInventoryActions_Drop(t *testing.T) {
	f := newFixture(t)
	s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)
	s.fillAndLock(t, f, s.playerProfile, map[int]int{s.coin: 20, s.sword: 1})

	content := fmt.Sprintf("She [item-drop items=%d:1,%d:5]emptied the bag of everything metal[/item-drop].", s.sword, s.coin)
	postID, code := s.publish(t, f, s.player, s.playerProfile, content)
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}
	if got := currentOf(t, f, s, s.playerProfile); got["Gold coin"] != 15 || len(got) != 1 {
		t.Errorf("expected only 15 coins left, got %v", got)
	}
	items, _ := Services.GetPostInventoryItems(f.db, postID)
	if got := quantities(items); got["Gold coin"] != 15 || len(got) != 1 {
		t.Errorf("the post's copy should show the state after the drop, got %v", got)
	}
}

func TestInventoryActions_Give(t *testing.T) {
	f := newFixture(t)
	s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)
	s.fillAndLock(t, f, s.playerProfile, map[int]int{s.coin: 20, s.sword: 1})
	s.fillAndLock(t, f, s.otherProfile, map[int]int{s.coin: 2})

	content := fmt.Sprintf("[item-give to=%d items=%d:1,%d:10]handed over the sword and half the purse[/item-give]", s.otherProfile, s.sword, s.coin)
	if _, code := s.publish(t, f, s.player, s.playerProfile, content); code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}
	if got := currentOf(t, f, s, s.playerProfile); got["Gold coin"] != 10 || len(got) != 1 {
		t.Errorf("giver should keep 10 coins only, got %v", got)
	}
	if got := currentOf(t, f, s, s.otherProfile); got["Gold coin"] != 12 || got["Sword"] != 1 {
		t.Errorf("recipient should hold 12 coins and the sword, got %v", got)
	}

	// The recipient's next post builds on what they received.
	if _, code := s.publish(t, f, s.other, s.otherProfile, fmt.Sprintf("[item-drop items=%d:12]x[/item-drop]", s.coin)); code != http.StatusCreated {
		t.Fatalf("recipient spending the gift: expected 201, got %d", code)
	}
	if got := currentOf(t, f, s, s.otherProfile); got["Sword"] != 1 || len(got) != 1 {
		t.Errorf("recipient should have only the sword left, got %v", got)
	}
}

func TestInventoryActions_Refused(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(f *fixture, s *invScene)
		content func(s *invScene) string
		ooc     bool
	}{
		{"not enough of an item", nil, func(s *invScene) string {
			return fmt.Sprintf("[item-drop items=%d:21]x[/item-drop]", s.coin)
		}, false},
		{"not enough once several actions add up", nil, func(s *invScene) string {
			return fmt.Sprintf("[item-drop items=%d:15]x[/item-drop][item-give to=%d items=%d:6]y[/item-give]", s.coin, s.otherProfile, s.coin)
		}, false},
		{"item not in the inventory", nil, func(s *invScene) string {
			return fmt.Sprintf("[item-drop items=%d:1]x[/item-drop]", s.sword)
		}, false},
		{"recipient not in the scene", func(f *fixture, s *invScene) {
			f.exec(t, "DELETE FROM episode_mask WHERE mask_id = ?", s.maskProfile)
		}, func(s *invScene) string {
			return fmt.Sprintf("[item-give to=%d items=%d:1]x[/item-give]", s.maskProfile, s.coin)
		}, false},
		{"recipient inventory not locked", func(f *fixture, s *invScene) {
			f.exec(t, "UPDATE inventory_scene SET is_locked = 0 WHERE character_profile_id = ?", s.otherProfile)
		}, func(s *invScene) string {
			return fmt.Sprintf("[item-give to=%d items=%d:1]x[/item-give]", s.otherProfile, s.coin)
		}, false},
		{"giving to yourself", nil, func(s *invScene) string {
			return fmt.Sprintf("[item-give to=%d items=%d:1]x[/item-give]", s.playerProfile, s.coin)
		}, false},
		{"author inventory not locked", func(f *fixture, s *invScene) {
			f.exec(t, "UPDATE inventory_scene SET is_locked = 0 WHERE character_profile_id = ?", s.playerProfile)
		}, func(s *invScene) string {
			return fmt.Sprintf("[item-drop items=%d:1]x[/item-drop]", s.coin)
		}, false},
		{"scene does not use inventory", func(f *fixture, s *invScene) {
			f.exec(t, "DELETE FROM inventory_episode_settings WHERE episode_id = ?", s.episodeID)
		}, func(s *invScene) string {
			return fmt.Sprintf("[item-drop items=%d:1]x[/item-drop]", s.coin)
		}, false},
		{"malformed action", nil, func(*invScene) string { return "[item-drop items=oops]x[/item-drop]" }, false},
		{"out-of-character post", nil, func(s *invScene) string {
			return fmt.Sprintf("[item-drop items=%d:1]x[/item-drop]", s.coin)
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)
			s.fillAndLock(t, f, s.playerProfile, map[int]int{s.coin: 20})
			s.fillAndLock(t, f, s.otherProfile, map[int]int{})
			if tt.setup != nil {
				tt.setup(f, s)
			}
			w := f.do(t, "POST", "/post/create", s.player, map[string]any{
				"topic_id": s.topicID, "content": tt.content(s), "use_character_profile": !tt.ooc, "character_profile_id": s.playerProfile,
			})
			expectStatus(t, w, http.StatusBadRequest)
			if n := f.count(t, "SELECT COUNT(*) FROM posts WHERE topic_id = ?", s.topicID); n != 0 {
				t.Errorf("a refused post must not be saved, found %d", n)
			}
			if n := f.count(t, "SELECT COUNT(*) FROM inventory_post"); n != 0 {
				t.Errorf("a refused post must not change inventories, found %d copies", n)
			}
		})
	}
}

func TestInventoryActions_PendingScene(t *testing.T) {
	f := newFixture(t)
	s := newInvScene(t, f, f.subA, Entities.PendingEpisode)
	s.fillAndLock(t, f, s.playerProfile, map[int]int{s.coin: 20})
	_, err := s.addActionPost(t, f, s.player, s.playerProfile, fmt.Sprintf("[item-drop items=%d:1]x[/item-drop]", s.coin))
	var actionErr *Services.InventoryActionError
	if !errors.As(err, &actionErr) {
		t.Errorf("expected actions in a pending scene to be refused, got %v", err)
	}
}

// The subforum permission check runs before any inventory work.
func TestInventoryActions_CreatePostPermissions(t *testing.T) {
	tests := []struct {
		name     string
		subforum func(f *fixture) int
		caller   func(f *fixture, s *invScene) int
	}{
		{"guest role with subforum permission", func(f *fixture) int { return f.subA }, func(f *fixture, _ *invScene) int { return f.guestUser }},
		{"player without subforum permission", func(f *fixture) int { return f.subB }, func(_ *fixture, s *invScene) int { return s.player }},
		{"admin with subforum permission, not the character's player", func(f *fixture) int { return f.subB }, func(f *fixture, _ *invScene) int { return f.admin }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			s := newInvScene(t, f, tt.subforum(f), Entities.ActiveEpisode)
			s.fillAndLock(t, f, s.playerProfile, map[int]int{s.coin: 20})
			w := f.do(t, "POST", "/post/create", tt.caller(f, s), map[string]any{
				"topic_id": s.topicID, "content": fmt.Sprintf("[item-drop items=%d:1]x[/item-drop]", s.coin),
				"use_character_profile": true, "character_profile_id": s.playerProfile,
			})
			expectStatus(t, w, http.StatusForbidden)
			if n := f.count(t, "SELECT COUNT(*) FROM inventory_post"); n != 0 {
				t.Errorf("expected no inventory change, found %d copies", n)
			}
		})
	}
}

func TestInventoryActions_Edit(t *testing.T) {
	drop := func(s *invScene, qty int, text string) string {
		return fmt.Sprintf("[item-drop items=%d:%d]%s[/item-drop]", s.coin, qty, text)
	}
	edit := func(t *testing.T, f *fixture, postID, userID int, content string) int {
		t.Helper()
		return f.do(t, "POST", fmt.Sprintf("/post/update/%d", postID), userID, map[string]any{"content": content}).Code
	}

	t.Run("latest post: changed actions are re-applied", func(t *testing.T) {
		f := newFixture(t)
		s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)
		s.fillAndLock(t, f, s.playerProfile, map[int]int{s.coin: 20})
		postID, _ := s.publish(t, f, s.player, s.playerProfile, drop(s, 5, "spent"))
		if code := edit(t, f, postID, s.player, drop(s, 8, "spent more")); code != http.StatusOK {
			t.Fatalf("expected 200, got %d", code)
		}
		if got := currentOf(t, f, s, s.playerProfile); got["Gold coin"] != 12 {
			t.Errorf("expected 20-8=12 coins, got %v", got)
		}
		if code := edit(t, f, postID, s.player, "changed my mind"); code != http.StatusOK {
			t.Fatalf("removing the action: expected 200, got %d", code)
		}
		if got := currentOf(t, f, s, s.playerProfile); got["Gold coin"] != 20 {
			t.Errorf("removing the action should restore 20 coins, got %v", got)
		}
	})

	t.Run("latest post: an impossible change is refused and nothing changes", func(t *testing.T) {
		f := newFixture(t)
		s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)
		s.fillAndLock(t, f, s.playerProfile, map[int]int{s.coin: 20})
		postID, _ := s.publish(t, f, s.player, s.playerProfile, drop(s, 5, "spent"))
		if code := edit(t, f, postID, s.player, drop(s, 50, "spent")); code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", code)
		}
		if got := currentOf(t, f, s, s.playerProfile); got["Gold coin"] != 15 {
			t.Errorf("expected 15 coins kept, got %v", got)
		}
		if n := f.count(t, "SELECT COUNT(*) FROM posts WHERE id = ? AND content LIKE '%:50]%'", postID); n != 0 {
			t.Error("the refused content must not be saved")
		}
	})

	t.Run("older post: text edits are fine, action changes are refused", func(t *testing.T) {
		f := newFixture(t)
		s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)
		s.fillAndLock(t, f, s.playerProfile, map[int]int{s.coin: 20})
		s.fillAndLock(t, f, s.otherProfile, map[int]int{})
		postID, _ := s.publish(t, f, s.player, s.playerProfile, drop(s, 5, "spent"))
		s.publish(t, f, s.other, s.otherProfile, "Bob answers.")

		if code := edit(t, f, postID, s.player, "Typo fixed. "+drop(s, 5, "spent")); code != http.StatusOK {
			t.Errorf("same actions with new text: expected 200, got %d", code)
		}
		if code := edit(t, f, postID, s.player, drop(s, 6, "spent")); code != http.StatusConflict {
			t.Errorf("changed action: expected 409, got %d", code)
		}
		if got := currentOf(t, f, s, s.playerProfile); got["Gold coin"] != 15 {
			t.Errorf("expected 15 coins kept, got %v", got)
		}
	})
}

func TestInventoryActions_Delete(t *testing.T) {
	setup := func(t *testing.T) (*fixture, *invScene, int) {
		f := newFixture(t)
		f.grantSubforum(t, roleUser, f.subA, "subforum_delete_own_post")
		s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)
		s.fillAndLock(t, f, s.playerProfile, map[int]int{s.coin: 20})
		s.fillAndLock(t, f, s.otherProfile, map[int]int{})
		postID, code := s.publish(t, f, s.player, s.playerProfile, fmt.Sprintf("[item-give to=%d items=%d:5]x[/item-give]", s.otherProfile, s.coin))
		if code != http.StatusCreated {
			t.Fatalf("publishing: %d", code)
		}
		return f, s, postID
	}

	t.Run("deleting the latest post undoes its actions", func(t *testing.T) {
		f, s, postID := setup(t)
		expectStatus(t, f.do(t, "POST", fmt.Sprintf("/post/delete/%d", postID), s.player, nil), http.StatusOK)
		if got := currentOf(t, f, s, s.playerProfile); got["Gold coin"] != 20 {
			t.Errorf("giver should be back to 20 coins, got %v", got)
		}
		if got := currentOf(t, f, s, s.otherProfile); len(got) != 0 {
			t.Errorf("recipient should be back to empty, got %v", got)
		}
	})

	t.Run("deleting an older post keeps its actions", func(t *testing.T) {
		f, s, postID := setup(t)
		s.publish(t, f, s.other, s.otherProfile, "Bob answers.")
		expectStatus(t, f.do(t, "POST", fmt.Sprintf("/post/delete/%d", postID), s.player, nil), http.StatusOK)
		if got := currentOf(t, f, s, s.playerProfile); got["Gold coin"] != 15 {
			t.Errorf("giver should keep 15 coins, got %v", got)
		}
		if got := currentOf(t, f, s, s.otherProfile); got["Gold coin"] != 5 {
			t.Errorf("recipient should keep 5 coins, got %v", got)
		}
	})
}

func TestInventoryActions_Rendering(t *testing.T) {
	f := newFixture(t)
	s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)
	f.exec(t, "UPDATE inventory_items SET name = '<b>Sword</b>' WHERE id = ?", s.sword)
	s.fillAndLock(t, f, s.playerProfile, map[int]int{s.coin: 20, s.sword: 1})
	s.fillAndLock(t, f, s.otherProfile, map[int]int{})
	content := fmt.Sprintf("[item-drop items=%d:1]tossed it[/item-drop] and [item-give to=%d items=%d:3]paid Bob[/item-give]",
		s.sword, s.otherProfile, s.coin)
	if _, code := s.publish(t, f, s.player, s.playerProfile, content); code != http.StatusCreated {
		t.Fatalf("publishing: %d", code)
	}

	t.Run("English, readable by another player", func(t *testing.T) {
		w := f.do(t, "GET", fmt.Sprintf("/topic-posts/%d", s.topicID), s.other, nil)
		expectStatus(t, w, http.StatusOK)
		html := decode[struct{ Posts []Entities.Post }](t, w.Body.Bytes()).Posts[0].ContentHtml
		for _, want := range []string{
			`<span class="inv-action inv-action--drop" tabindex="0" data-tip="Dropped: &lt;b&gt;Sword&lt;/b&gt; ×1">tossed it</span>`,
			`<span class="inv-action inv-action--give" tabindex="0" data-tip="Given to Bob: Gold coin ×3">paid Bob</span>`,
		} {
			if !strings.Contains(html, want) {
				t.Errorf("expected %s in %s", want, html)
			}
		}
	})

	t.Run("in the viewer's language", func(t *testing.T) {
		f.exec(t, "UPDATE users SET interface_language = 'ru' WHERE id = ?", s.other)
		w := f.do(t, "GET", fmt.Sprintf("/topic-posts/%d", s.topicID), s.other, nil)
		html := decode[struct{ Posts []Entities.Post }](t, w.Body.Bytes()).Posts[0].ContentHtml
		if !strings.Contains(html, "Выброшено: ") || !strings.Contains(html, "Bob получает: ") {
			t.Errorf("expected Russian labels in %s", html)
		}
	})

	t.Run("preview renders actions too", func(t *testing.T) {
		w := f.do(t, "POST", "/post/preview", s.player, Controllers.CreatePostRequest{TopicID: s.topicID, Content: content})
		expectStatus(t, w, http.StatusOK)
		if html := decode[Entities.Post](t, w.Body.Bytes()).ContentHtml; !strings.Contains(html, "Given to Bob: Gold coin ×3") {
			t.Errorf("expected the give hover in the preview, got %s", html)
		}
	})
}

func TestInventoryRecipients(t *testing.T) {
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
			s.fillAndLock(t, f, s.playerProfile, map[int]int{})
			s.fillAndLock(t, f, s.maskProfile, map[int]int{})
			f.insert(t, "INSERT INTO inventory_scene (episode_id, character_profile_id, is_locked) VALUES (?, ?, 0)", s.episodeID, s.otherProfile)

			w := f.do(t, "GET", fmt.Sprintf("/episode/%d/inventory/recipients", s.episodeID), tt.caller(f, s), nil)
			expectStatus(t, w, tt.want)
			if tt.want != http.StatusOK {
				return
			}
			var names []string
			for _, r := range decode[[]Entities.InventoryRecipient](t, w.Body.Bytes()) {
				names = append(names, r.Name)
			}
			// Bob's inventory is not locked, so he can't receive anything yet.
			if got := strings.Join(names, ","); got != "Alice,The Stranger" {
				t.Errorf("expected Alice and The Stranger, got %q", got)
			}
		})
	}

	t.Run("scene without inventory", func(t *testing.T) {
		f := newFixture(t)
		s := newInvScene(t, f, f.subA, Entities.ActiveEpisode)
		f.exec(t, "DELETE FROM inventory_episode_settings WHERE episode_id = ?", s.episodeID)
		expectStatus(t, f.do(t, "GET", fmt.Sprintf("/episode/%d/inventory/recipients", s.episodeID), s.player, nil), http.StatusConflict)
	})
}

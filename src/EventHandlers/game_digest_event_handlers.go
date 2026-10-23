package EventHandlers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Events"
	"cuento-backend/src/MCP"
	"database/sql"
	"encoding/json"
	"log"
)

func RegisterGameDigestEventHandlers() {
	// Summarize new episode posts for the weekly digest.
	Events.Subscribe(Events.PostCreated, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.PostCreatedEvent)
		if !ok || event.Type != "post_created" {
			return
		}

		var topicType Entities.TopicType
		if err := db.QueryRow("SELECT type FROM topics WHERE id = ?", event.TopicID).Scan(&topicType); err != nil || topicType != Entities.EpisodeTopic {
			return
		}

		MCP.SummarizeEpisodePost(db, event.Post.Id, event.TopicID, event.SubforumID)
	})

	// Mark the weekly note as deleted when a post is soft-deleted.
	Events.Subscribe(Events.PostCreated, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.PostCreatedEvent)
		if !ok || event.Type != "post_deleted" {
			return
		}

		if _, err := db.Exec(
			`UPDATE game_digest_weekly_post_notes SET is_deleted = 1 WHERE post_id = ?`,
			event.Post.Id,
		); err != nil {
			log.Printf("game_digest: failed to mark post %d as deleted: %v", event.Post.Id, err)
		}
	})

	// Seed episode notes when a new episode is created.
	Events.Subscribe(Events.EpisodeCreated, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.EpisodeCreatedEvent)
		if !ok {
			return
		}

		notes := buildInitialEpisodeNotes(db, event.EpisodeID, event.TopicName)
		notesJSON, err := json.Marshal(notes)
		if err != nil {
			log.Printf("game_digest: failed to marshal episode notes for episode %d: %v", event.EpisodeID, err)
			return
		}

		if _, err := db.Exec(
			`INSERT INTO game_digest_episode_notes (episode_id, notes)
			 VALUES (?, ?)
			 ON DUPLICATE KEY UPDATE notes = VALUES(notes)`,
			event.EpisodeID, string(notesJSON),
		); err != nil {
			log.Printf("game_digest: failed to save initial notes for episode %d: %v", event.EpisodeID, err)
		}
	})
}

type initialEpisodeNotes struct {
	Name       string            `json:"name"`
	Fields     map[string]string `json:"fields"`
	Characters []string          `json:"characters"`
	Factions   []string          `json:"factions"`
}

func buildInitialEpisodeNotes(db *sql.DB, episodeID int64, topicName string) initialEpisodeNotes {
	notes := initialEpisodeNotes{
		Name:       topicName,
		Fields:     map[string]string{},
		Characters: []string{},
		Factions:   []string{},
	}

	// Episode header fields (location, timeline, in-world date, etc.).
	rows, err := db.Query(`
		SELECT field_machine_name,
		       COALESCE(value_string, value_text, CAST(value_int AS CHAR), value_date, '') AS value
		FROM episode_main
		WHERE entity_id = ?
		  AND field_machine_name IS NOT NULL
		  AND (value_string IS NOT NULL OR value_text IS NOT NULL OR value_int IS NOT NULL OR value_date IS NOT NULL)`,
		episodeID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err == nil && v != "" {
				notes.Fields[k] = v
			}
		}
	}

	// Participating characters.
	charRows, err := db.Query(`
		SELECT cb.name
		FROM episode_character ec
		JOIN character_base cb ON cb.id = ec.character_id
		WHERE ec.episode_id = ?`, episodeID)
	if err == nil {
		defer charRows.Close()
		for charRows.Next() {
			var name string
			if err := charRows.Scan(&name); err == nil {
				notes.Characters = append(notes.Characters, name)
			}
		}
	}

	// Factions represented in this episode (via the characters).
	if len(notes.Characters) > 0 {
		factionRows, err := db.Query(`
			SELECT DISTINCT f.name
			FROM episode_character ec
			JOIN character_faction cf ON cf.character_id = ec.character_id
			JOIN factions f ON f.id = cf.faction_id
			WHERE ec.episode_id = ?`, episodeID)
		if err == nil {
			defer factionRows.Close()
			for factionRows.Next() {
				var name string
				if err := factionRows.Scan(&name); err == nil {
					notes.Factions = append(notes.Factions, name)
				}
			}
		}
	}

	// Override name from episode_base if set (episode_base.name can differ from topic name).
	var episodeName sql.NullString
	if err := db.QueryRow(`SELECT name FROM episode_base WHERE id = ?`, episodeID).Scan(&episodeName); err == nil && episodeName.Valid && episodeName.String != "" {
		notes.Name = episodeName.String
	}

	if len(notes.Characters) == 0 {
		notes.Characters = []string{}
	}
	if len(notes.Factions) == 0 {
		notes.Factions = []string{}
	}

	return notes
}

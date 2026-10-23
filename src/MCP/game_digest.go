package MCP

import (
	"context"
	"cuento-backend/src/Entities"
	"cuento-backend/src/Events"
	"cuento-backend/src/Services"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const gameDigestInstructions = `You are the Game Digest agent for a roleplay forum. Your task is to write a structured weekly digest from a set of episode summaries.

You will receive a list of episodes. Each episode entry includes:
- Its name
- Custom fields set by the players: in-game date, time, location, and any other header fields
- The factions whose characters participate in this episode
- Short summaries of individual posts from this episode during the period
- A previous context note describing what happened before this period (if any)

Format rules:
1. Organise the digest into sections — one per faction. Use [b]Faction Name[/b] as the section header.
   If an episode involves characters from multiple factions, include a short mention in each relevant faction's section.
   Episodes with no clear faction alignment go into a separate [b]General[/b] section.
2. Within a faction section, write one concise paragraph per episode (2–4 sentences) that synthesises the post summaries into a coherent in-world account. Do not retell individual posts — write as a unified narrative.
3. If two or more episodes in the same section share the same in-game time period (same date, approximate time, or same named moment), group them under a single time marker formatted as [i]time, place[/i] and write one combined paragraph.
4. Use the in-game date, time, and location from each episode's custom fields as the primary source for the time marker. If those fields are absent, omit the marker.
5. Write in the specified output language.
6. Do not mention usernames, post counts, or any out-of-character details.
7. Do not invent events — only use what the summaries describe.
8. If there are no meaningful events for a faction, omit that section entirely.

Return only the BB-formatted digest text, no commentary.`

type gameDigestConfig struct {
	Period        string `json:"period"`
	SubforumIDs   []int  `json:"subforum_ids"`
	FactionIDs    []int  `json:"faction_ids"`
	TargetTopicID *int   `json:"target_topic_id"`
	Language      string `json:"language"`
}

type digestHeaderField struct {
	MachineName string
	Value       string
}

type weeklyEpisodeData struct {
	EpisodeID    int64
	TopicID      int64
	TopicName    string
	HeaderFields []digestHeaderField
	Factions     []string
	Summaries    []string
	PrevContext  string
}

func executeGameDigestTask(db *sql.DB, taskID int) {
	ctx := context.Background()

	client, err := getActivePool().ClientForMinSize(Entities.AIModelSizeMedium, Entities.AIModelTypeText)
	if err != nil {
		markFailed(db, taskID, 0, "no AI model available: "+err.Error())
		return
	}

	var payloadStr string
	if err := db.QueryRow(`SELECT COALESCE(payload, '{}') FROM ai_task_queue WHERE id = ?`, taskID).Scan(&payloadStr); err != nil {
		markFailed(db, taskID, 0, "failed to read payload: "+err.Error())
		return
	}
	var payload struct {
		ImplementationID int `json:"implementation_id"`
	}
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil || payload.ImplementationID == 0 {
		markFailed(db, taskID, 0, "invalid payload")
		return
	}

	var configStr sql.NullString
	if err := db.QueryRow(`SELECT config FROM ai_agent_implementation WHERE id = ?`, payload.ImplementationID).Scan(&configStr); err != nil {
		markFailed(db, taskID, 0, "implementation not found")
		return
	}
	var config gameDigestConfig
	if configStr.Valid {
		_ = json.Unmarshal([]byte(configStr.String), &config)
	}
	if config.Period == "" {
		config.Period = "weekly"
	}

	since := periodToTime(config.Period)

	episodes, err := loadWeeklyEpisodes(db, config, since, payload.ImplementationID)
	if err != nil {
		markFailed(db, taskID, 0, "failed to load weekly episodes: "+err.Error())
		return
	}
	if len(episodes) == 0 {
		_, _ = db.Exec(`UPDATE ai_task_queue SET status = 'done', date_completed = NOW() WHERE id = ?`, taskID)
		return
	}

	userMsg := buildWeeklyDigestPrompt(config.Language, episodes)
	var digest string
	err = withModelRetry(
		getActivePool(),
		Entities.AIModelSizeMedium, Entities.AIModelTypeText,
		client,
		func(c AIClient) error {
			var e error
			digest, _, e = NewAIAgent(c).Chat(ctx, []ChatMessage{{Role: "user", Content: userMsg}}, gameDigestInstructions)
			return e
		},
		nil,
	)
	if err != nil {
		markFailed(db, taskID, 0, "AI call failed: "+err.Error())
		return
	}

	if config.TargetTopicID != nil {
		res, err := db.Exec(
			`INSERT INTO posts (topic_id, author_user_id, content, date_created, use_character_profile) VALUES (?, 1, ?, NOW(), 0)`,
			*config.TargetTopicID, digest,
		)
		if err == nil {
			postID, _ := res.LastInsertId()
			var subforumID int
			_ = db.QueryRow(`SELECT subforum_id FROM topics WHERE id = ?`, *config.TargetTopicID).Scan(&subforumID)
			if fullPost, err := Services.GetPostById(int(postID), db, false); err == nil {
				Events.Publish(db, Events.PostCreated, Events.PostCreatedEvent{
					Type:       "post_created",
					TopicID:    int64(*config.TargetTopicID),
					SubforumID: subforumID,
					Post:       *fullPost,
				})
			}

			_, _ = db.Exec(
				`INSERT INTO game_digests (implementation_id, datetime, text, is_published, published_post_id) VALUES (?, NOW(), ?, 1, ?)`,
				payload.ImplementationID, digest, postID,
			)
		}
	} else {
		_, _ = db.Exec(
			`INSERT INTO game_digests (implementation_id, datetime, text, is_published) VALUES (?, NOW(), ?, 0)`,
			payload.ImplementationID, digest,
		)
	}

	// Update per-episode context with the summaries covered this period.
	for i := range episodes {
		ep := &episodes[i]
		if len(ep.Summaries) > 0 {
			updateDigestContext(db, payload.ImplementationID, ep.TopicID, strings.Join(ep.Summaries, "\n"))
		}
	}

	_, _ = db.Exec(`UPDATE ai_task_queue SET status = 'done', date_completed = NOW() WHERE id = ?`, taskID)
}

// loadWeeklyEpisodes returns all episodes that have non-deleted post summaries since `since`,
// filtered by the config's subforum and faction lists.
func loadWeeklyEpisodes(db *sql.DB, config gameDigestConfig, since time.Time, implementationID int) ([]weeklyEpisodeData, error) {
	query := `
		SELECT DISTINCT eb.id, t.id, t.name
		FROM game_digest_weekly_post_notes wpn
		JOIN posts p ON p.id = wpn.post_id
		JOIN topics t ON t.id = p.topic_id
		JOIN episode_base eb ON eb.topic_id = t.id
		WHERE wpn.is_deleted = 0
		  AND wpn.is_complete = 1
		  AND wpn.post_summary IS NOT NULL AND wpn.post_summary != ''
		  AND p.date_created >= ?
		  AND t.status != ?
		  AND t.type = ?`

	args := []interface{}{since, Entities.DeletedTopic, Entities.EpisodeTopic}

	if len(config.SubforumIDs) > 0 {
		ph := strings.Repeat("?,", len(config.SubforumIDs)-1) + "?"
		query += " AND t.subforum_id IN (" + ph + ")"
		for _, id := range config.SubforumIDs {
			args = append(args, id)
		}
	}

	if len(config.FactionIDs) > 0 {
		ph := strings.Repeat("?,", len(config.FactionIDs)-1) + "?"
		query += ` AND EXISTS (
			SELECT 1 FROM episode_character ec
			JOIN character_faction cf ON cf.character_id = ec.character_id
			WHERE ec.episode_id = eb.id AND cf.faction_id IN (` + ph + `))`
		for _, id := range config.FactionIDs {
			args = append(args, id)
		}
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var episodes []weeklyEpisodeData
	for rows.Next() {
		var ep weeklyEpisodeData
		if err := rows.Scan(&ep.EpisodeID, &ep.TopicID, &ep.TopicName); err != nil {
			continue
		}
		episodes = append(episodes, ep)
	}

	for i := range episodes {
		ep := &episodes[i]
		ep.HeaderFields = loadEpisodeHeaderFields(db, ep.EpisodeID)
		ep.Factions = loadEpisodeFactions(db, ep.EpisodeID)
		ep.Summaries = loadWeeklyPostSummaries(db, ep.TopicID, since)
		ep.PrevContext = loadDigestContext(db, implementationID, ep.TopicID)
	}

	return episodes, nil
}

// loadEpisodeFactions returns the names of all factions represented in the episode.
func loadEpisodeFactions(db *sql.DB, episodeID int64) []string {
	rows, err := db.Query(`
		SELECT DISTINCT f.name
		FROM episode_character ec
		JOIN character_faction cf ON cf.character_id = ec.character_id
		JOIN factions f ON f.id = cf.faction_id
		WHERE ec.episode_id = ?`, episodeID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			names = append(names, name)
		}
	}
	return names
}

// loadWeeklyPostSummaries returns all non-deleted post summaries for a topic since `since`.
func loadWeeklyPostSummaries(db *sql.DB, topicID int64, since time.Time) []string {
	rows, err := db.Query(`
		SELECT wpn.post_summary
		FROM game_digest_weekly_post_notes wpn
		JOIN posts p ON p.id = wpn.post_id
		WHERE p.topic_id = ?
		  AND wpn.is_deleted = 0
		  AND wpn.is_complete = 1
		  AND wpn.post_summary IS NOT NULL AND wpn.post_summary != ''
		  AND p.date_created >= ?
		ORDER BY p.date_created ASC`, topicID, since)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var summaries []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err == nil {
			summaries = append(summaries, s)
		}
	}
	return summaries
}

// buildWeeklyDigestPrompt assembles the user message sent to the AI.
func buildWeeklyDigestPrompt(language string, episodes []weeklyEpisodeData) string {
	var sb strings.Builder

	if language != "" {
		sb.WriteString(fmt.Sprintf("Output language: %s\n\n", language))
	}

	sb.WriteString("## Episodes this period\n\n")

	for _, ep := range episodes {
		sb.WriteString(fmt.Sprintf("### %s\n", ep.TopicName))

		if len(ep.Factions) > 0 {
			sb.WriteString(fmt.Sprintf("Factions: %s\n", strings.Join(ep.Factions, ", ")))
		}

		for _, f := range ep.HeaderFields {
			sb.WriteString(fmt.Sprintf("%s: %s\n", f.MachineName, f.Value))
		}

		if ep.PrevContext != "" {
			sb.WriteString(fmt.Sprintf("Previous context: %s\n", ep.PrevContext))
		}

		sb.WriteString("Post summaries:\n")
		for _, s := range ep.Summaries {
			sb.WriteString(fmt.Sprintf("- %s\n", s))
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

// ── Unchanged utility functions ────────────────────────────────────────────────

func loadEpisodeHeaderFields(db *sql.DB, episodeID int64) []digestHeaderField {
	rows, err := db.Query(`
		SELECT field_machine_name,
		       COALESCE(value_string, value_text, CAST(value_int AS CHAR), value_date, '') AS value
		FROM episode_main
		WHERE entity_id = ? AND field_machine_name IS NOT NULL AND (
		      value_string IS NOT NULL OR value_text IS NOT NULL OR
		      value_int IS NOT NULL OR value_date IS NOT NULL
		)`, episodeID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var fields []digestHeaderField
	for rows.Next() {
		var f digestHeaderField
		if err := rows.Scan(&f.MachineName, &f.Value); err == nil && f.Value != "" {
			fields = append(fields, f)
		}
	}
	return fields
}

func loadDigestContext(db *sql.DB, implementationID int, topicID int64) string {
	var ctx sql.NullString
	_ = db.QueryRow(
		`SELECT context_text FROM game_digest_context WHERE implementation_id = ? AND topic_id = ?`,
		implementationID, topicID,
	).Scan(&ctx)
	if ctx.Valid {
		return ctx.String
	}
	return ""
}

func updateDigestContext(db *sql.DB, implementationID int, topicID int64, contextText string) {
	_, _ = db.Exec(`
		INSERT INTO game_digest_context (implementation_id, topic_id, context_text, date_updated)
		VALUES (?, ?, ?, NOW())
		ON DUPLICATE KEY UPDATE context_text = VALUES(context_text), date_updated = NOW()`,
		implementationID, topicID, contextText,
	)
}

func periodToTime(period string) time.Time {
	now := time.Now()
	switch period {
	case "daily":
		return now.Add(-24 * time.Hour)
	case "bi-weekly":
		return now.Add(-14 * 24 * time.Hour)
	case "monthly":
		return now.Add(-30 * 24 * time.Hour)
	default: // weekly
		return now.Add(-7 * 24 * time.Hour)
	}
}


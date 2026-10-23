package MCP

import (
	"context"
	"cuento-backend/src/Entities"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

const postSummarySystemInstruction = `You are the Game Digest assistant for a roleplay forum. Your task is to write a concise in-world summary of a single roleplay post.

You will receive:
- The output language for the summary
- The episode context: in-world date, location, and other header fields set by the players
- General notes about this episode (may include character name translations and other useful context)
- A list of characters participating in this episode
- The full text of the post to summarize

Your task:
1. Read the post and summarize the key in-world events in 2–4 sentences.
2. Write in the specified output language. If character names appear in a different language or alphabet, use the translated forms from the episode notes if available.
3. Do not mention usernames, post counts, or any out-of-character details.
4. Do not invent events — only summarize what the post actually describes.
5. If you encounter something you cannot determine from the provided data (for example, a character appears whose name translation is not in the episode notes, or a location is referenced that is ambiguous), add a specific question to pending_questions.

Return your response as a JSON object with exactly this structure:
{"summary": "<in-world summary in the specified language>", "pending_questions": ["<question 1>", "<question 2>"]}
If there are no pending questions, return an empty array.
Return only the raw JSON object — no markdown fences, no commentary.`

type postSummaryResult struct {
	Summary          string   `json:"summary"`
	PendingQuestions []string `json:"pending_questions"`
}

type postSummaryModelRequirements struct {
	MinSize int `json:"min_size"`
}

// SummarizeEpisodePost is called on PostCreated. It checks the feature settings,
// builds an episode context prompt, calls the AI, and writes the result to
// game_digest_weekly_post_notes.
func SummarizeEpisodePost(db *sql.DB, postID int, topicID int64, subforumID int) {
	pool := getActivePool()
	if pool == nil {
		return
	}

	// 1. Load all relevant settings in one pass.
	excludedSubforums, summaryLanguage, modelReq := loadPostSummarySettings(db)

	// 2. Skip if this subforum is excluded.
	for _, id := range excludedSubforums {
		if id == subforumID {
			return
		}
	}

	// 3. Resolve episode from the topic.
	var episodeID int64
	if err := db.QueryRow(`SELECT id FROM episode_base WHERE topic_id = ?`, topicID).Scan(&episodeID); err != nil {
		// Not an episode topic or episode not found — nothing to do.
		return
	}

	// 4. Pick AI model.
	minSize := Entities.AIModelSize(modelReq.MinSize)
	if minSize < Entities.AIModelSizeTiny || minSize > Entities.AIModelSizeLarge {
		minSize = Entities.AIModelSizeSmall
	}
	client, err := pool.ClientForMinSize(minSize, Entities.AIModelTypeText)
	if err != nil {
		log.Printf("game_digest post summary: no AI model available: %v", err)
		return
	}

	// 5. Load post content.
	var postContent string
	if err := db.QueryRow(`SELECT content FROM posts WHERE id = ?`, postID).Scan(&postContent); err != nil {
		log.Printf("game_digest post summary: failed to load post %d: %v", postID, err)
		return
	}

	// 6. Build the user message with episode context.
	userMsg := buildPostSummaryPrompt(db, episodeID, topicID, summaryLanguage, postContent)

	// 7. Call the AI with retry on rate-limit.
	var rawReply string
	err = withModelRetry(
		pool,
		minSize, Entities.AIModelTypeText,
		client,
		func(c AIClient) error {
			var e error
			rawReply, _, e = c.Chat(context.Background(), []ChatMessage{
				{Role: "user", Content: userMsg},
			}, postSummarySystemInstruction)
			return e
		},
		nil,
	)
	if err != nil {
		log.Printf("game_digest post summary: AI call failed for post %d: %v", postID, err)
		return
	}

	// 8. Parse JSON response.
	rawReply = stripMarkdownFences(rawReply)
	var result postSummaryResult
	if err := json.Unmarshal([]byte(rawReply), &result); err != nil {
		log.Printf("game_digest post summary: failed to parse AI response for post %d: %v", postID, err)
		return
	}

	// 9. Compute Monday of the post's week.
	var postDate time.Time
	if err := db.QueryRow(`SELECT date_created FROM posts WHERE id = ?`, postID).Scan(&postDate); err != nil {
		postDate = time.Now()
	}
	weekStart := mondayOf(postDate)

	// 10. Persist to game_digest_weekly_post_notes.
	pendingJSON, _ := json.Marshal(result.PendingQuestions)
	_, err = db.Exec(`
		INSERT INTO game_digest_weekly_post_notes
			(post_id, week_start, is_deleted, is_complete, post_summary, pending_questions)
		VALUES (?, ?, 0, 0, ?, ?)
		ON DUPLICATE KEY UPDATE
			post_summary      = VALUES(post_summary),
			pending_questions = VALUES(pending_questions),
			is_complete       = (JSON_LENGTH(VALUES(pending_questions)) = 0)`,
		postID, weekStart.Format("2006-01-02"), result.Summary, string(pendingJSON),
	)
	if err != nil {
		log.Printf("game_digest post summary: failed to save note for post %d: %v", postID, err)
		return
	}

	// 11. If there are pending questions, notify the digest manager via AI chat.
	if len(result.PendingQuestions) > 0 {
		notifyUserID := loadNotifyUserID(db)
		if notifyUserID > 0 {
			go postDigestPendingQuestionsToChat(db, notifyUserID, postID, episodeID, result.PendingQuestions)
		}
	}
}

// loadPostSummarySettings reads the three settings used by SummarizeEpisodePost.
func loadPostSummarySettings(db *sql.DB) (excludedSubforums []int, summaryLanguage string, modelReq postSummaryModelRequirements) {
	modelReq.MinSize = int(Entities.AIModelSizeSmall)

	rows, err := db.Query(`SELECT setting_name, setting_value FROM game_digest_settings WHERE setting_name IN ('excluded_subforums', 'summary_language', 'model_requirements')`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var val sql.NullString
		if err := rows.Scan(&name, &val); err != nil || !val.Valid {
			continue
		}
		switch name {
		case "excluded_subforums":
			_ = json.Unmarshal([]byte(val.String), &excludedSubforums)
		case "summary_language":
			_ = json.Unmarshal([]byte(val.String), &summaryLanguage)
		case "model_requirements":
			_ = json.Unmarshal([]byte(val.String), &modelReq)
		}
	}
	return
}

// buildPostSummaryPrompt assembles the user message sent to the AI.
func buildPostSummaryPrompt(db *sql.DB, episodeID, topicID int64, summaryLanguage, postContent string) string {
	var sb strings.Builder

	if summaryLanguage != "" {
		sb.WriteString(fmt.Sprintf("Output language: %s\n\n", summaryLanguage))
	}

	// Episode header fields (date, location, etc.)
	headerFields := loadEpisodeHeaderFields(db, episodeID)
	if len(headerFields) > 0 {
		sb.WriteString("## Episode context\n")
		for _, f := range headerFields {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", f.MachineName, f.Value))
		}
		sb.WriteString("\n")
	}

	// General episode notes stored by admins (may include character name translations).
	var episodeNotes sql.NullString
	var additionalContext sql.NullString
	_ = db.QueryRow(`SELECT notes, additional_context FROM game_digest_episode_notes WHERE episode_id = ?`, episodeID).Scan(&episodeNotes, &additionalContext)
	if episodeNotes.Valid && episodeNotes.String != "" && episodeNotes.String != "null" {
		sb.WriteString("## Episode notes\n")
		sb.WriteString(episodeNotes.String)
		sb.WriteString("\n\n")
	}
	if additionalContext.Valid && additionalContext.String != "" {
		sb.WriteString("## Additional context\n")
		sb.WriteString(additionalContext.String)
		sb.WriteString("\n\n")
	}

	// Characters participating in this episode.
	characters := loadEpisodeCharacterNames(db, episodeID)
	if len(characters) > 0 {
		sb.WriteString("## Participating characters\n")
		for _, name := range characters {
			sb.WriteString(fmt.Sprintf("- %s\n", name))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Post to summarize\n\n")
	sb.WriteString(postContent)

	return sb.String()
}

// loadEpisodeCharacterNames returns the display names of all characters in the episode
// (mask name → character name → username fallback).
func loadEpisodeCharacterNames(db *sql.DB, episodeID int64) []string {
	rows, err := db.Query(`
		SELECT COALESCE(cp.mask_name, cb.name, u.username)
		FROM episode_character ec
		JOIN character_base cb ON cb.id = ec.character_id
		LEFT JOIN users u ON u.id = cb.user_id
		LEFT JOIN character_profile_base cp ON cp.character_id = cb.id
		WHERE ec.episode_id = ?
		GROUP BY cb.id`, episodeID)
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

// mondayOf returns midnight on the Monday of the week containing t.
func mondayOf(t time.Time) time.Time {
	weekday := t.Weekday()
	daysFromMonday := int(weekday) - int(time.Monday)
	if daysFromMonday < 0 {
		daysFromMonday += 7
	}
	d := t.AddDate(0, 0, -daysFromMonday)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
}

// stripMarkdownFences removes accidental ```json … ``` wrappers from model output.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	return s
}

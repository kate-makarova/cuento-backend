package MCP

import (
	"context"
	"cuento-backend/src/Entities"
	"cuento-backend/src/Events"
	"cuento-backend/src/Websockets"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

const digestAnswerResolverInstruction = `You are the Game Digest assistant. The game master has answered questions about their game's roleplay posts that you could not summarize from available data. Extract the relevant answers and prepare concise notes additions per episode.

You will receive:
- The pending questions grouped by episode
- The game master's response

Your task:
1. Match each question to the answer in the response.
2. For each episode that has answers, produce a notes_addition: a short paragraph with only the newly clarified information (e.g. character name translations, location clarifications). Write it in the same language as the questions.
3. Write a short, friendly user_reply confirming what you understood and that the affected posts will be re-summarized.

Return a JSON object with exactly this structure:
{"user_reply": "<confirmation>", "episode_updates": [{"episode_id": <integer>, "notes_addition": "<text>"}]}
Return only the raw JSON — no markdown fences.`

type pendingAnswerRequest struct {
	PostID    int
	EpisodeID int64
	Questions []string
}

type digestAnswerResult struct {
	UserReply      string `json:"user_reply"`
	EpisodeUpdates []struct {
		EpisodeID     int64  `json:"episode_id"`
		NotesAddition string `json:"notes_addition"`
	} `json:"episode_updates"`
}

// postDigestPendingQuestionsToChat injects an AI chat message for the post author
// listing questions the summariser could not answer.
func postDigestPendingQuestionsToChat(db *sql.DB, notifyUserID int, postID int, episodeID int64, questions []string) {
	var episodeName string
	_ = db.QueryRow(
		`SELECT COALESCE(eb.name, t.name, '') FROM episode_base eb JOIN topics t ON t.id = eb.topic_id WHERE eb.id = ?`,
		episodeID,
	).Scan(&episodeName)
	if episodeName == "" {
		episodeName = fmt.Sprintf("episode %d", episodeID)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("I was summarizing posts in the episode \"%s\" and encountered some details I couldn't determine from the available data:\n\n", episodeName))
	for i, q := range questions {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, q))
	}
	sb.WriteString("\nCould you provide these details so I can complete the summaries?")
	content := sb.String()

	r, err := db.Exec(
		`INSERT INTO ai_chat_messages (user_id, role, content, date_created) VALUES (?, 'assistant', ?, NOW())`,
		notifyUserID, content,
	)
	if err != nil {
		log.Printf("game_digest pending: failed to insert chat message: %v", err)
		return
	}
	msgID, _ := r.LastInsertId()

	msg := Entities.AIChatMessage{
		Id:          int(msgID),
		UserId:      notifyUserID,
		Role:        "assistant",
		Content:     content,
		DateCreated: time.Now(),
	}
	Websockets.MainHub.SendNotification(notifyUserID, map[string]interface{}{
		"type": "ai_message",
		"data": msg,
	})
	Events.Publish(db, Events.NotificationCreated, Events.NotificationEvent{
		UserID:  notifyUserID,
		Type:    "ai_message",
		Message: "The AI has questions about the game digest summaries",
		Data:    Entities.NotificationAiMessage{},
	})
}

// loadPendingDigestRequests returns all posts authored by userID that have unresolved pending questions.
func loadPendingDigestRequests(db *sql.DB, userID int) []pendingAnswerRequest {
	rows, err := db.Query(`
		SELECT wpn.post_id, eb.id, wpn.pending_questions
		FROM game_digest_weekly_post_notes wpn
		JOIN posts p ON p.id = wpn.post_id
		JOIN topics t ON t.id = p.topic_id
		JOIN episode_base eb ON eb.topic_id = t.id
		WHERE p.user_id = ?
		  AND wpn.is_deleted = 0
		  AND wpn.is_complete = 0
		  AND wpn.pending_questions IS NOT NULL
		  AND JSON_LENGTH(wpn.pending_questions) > 0
		ORDER BY p.date_created ASC`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var requests []pendingAnswerRequest
	for rows.Next() {
		var req pendingAnswerRequest
		var questionsJSON string
		if err := rows.Scan(&req.PostID, &req.EpisodeID, &questionsJSON); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(questionsJSON), &req.Questions)
		requests = append(requests, req)
	}
	return requests
}

func executeDigestAnswerResolver(db *sql.DB, taskID, userID int, userMsgContent string, requests []pendingAnswerRequest) {
	pool := getActivePool()
	if pool == nil {
		markFailed(db, taskID, userID, "no AI pool available")
		return
	}
	client, err := pool.ClientForMinSize(Entities.AIModelSizeSmall, Entities.AIModelTypeText)
	if err != nil {
		markFailed(db, taskID, userID, "no AI model available: "+err.Error())
		return
	}

	// Group requests by episode for the prompt.
	type episodeGroup struct {
		EpisodeID int64
		Name      string
		PostIDs   []int
		Questions []string
	}
	episodeMap := map[int64]*episodeGroup{}
	var episodeOrder []int64
	for _, req := range requests {
		if _, ok := episodeMap[req.EpisodeID]; !ok {
			var name string
			_ = db.QueryRow(
				`SELECT COALESCE(eb.name, t.name, '') FROM episode_base eb JOIN topics t ON t.id = eb.topic_id WHERE eb.id = ?`,
				req.EpisodeID,
			).Scan(&name)
			if name == "" {
				name = fmt.Sprintf("Episode %d", req.EpisodeID)
			}
			episodeMap[req.EpisodeID] = &episodeGroup{EpisodeID: req.EpisodeID, Name: name}
			episodeOrder = append(episodeOrder, req.EpisodeID)
		}
		g := episodeMap[req.EpisodeID]
		g.PostIDs = append(g.PostIDs, req.PostID)
		g.Questions = append(g.Questions, req.Questions...)
	}

	var sb strings.Builder
	sb.WriteString("The game master has answered questions about pending game digest summaries.\n\n")
	sb.WriteString("## Pending questions by episode\n\n")
	for _, eid := range episodeOrder {
		g := episodeMap[eid]
		sb.WriteString(fmt.Sprintf("### %s (episode_id: %d)\n", g.Name, g.EpisodeID))
		for i, q := range g.Questions {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, q))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("## Game master's response\n\n")
	sb.WriteString(userMsgContent)

	var rawReply string
	err = withModelRetry(
		pool,
		Entities.AIModelSizeSmall, Entities.AIModelTypeText,
		client,
		func(c AIClient) error {
			var e error
			rawReply, _, e = c.Chat(context.Background(), []ChatMessage{
				{Role: "user", Content: sb.String()},
			}, digestAnswerResolverInstruction)
			return e
		},
		func(modelID string) {
			_, _ = db.Exec(`UPDATE ai_task_queue SET retries = retries + 1 WHERE id = ?`, taskID)
			log.Printf("game_digest resolver: task %d rate-limited on %s, trying next model", taskID, modelID)
		},
	)
	if err != nil {
		markFailed(db, taskID, userID, "AI call failed: "+err.Error())
		return
	}

	rawReply = stripMarkdownFences(rawReply)
	var result digestAnswerResult
	if err := json.Unmarshal([]byte(rawReply), &result); err != nil {
		markFailed(db, taskID, userID, "failed to parse AI response: "+err.Error())
		return
	}

	// Apply episode note additions, clear pending_questions, and re-trigger summarization.
	for _, update := range result.EpisodeUpdates {
		if update.NotesAddition == "" {
			continue
		}
		appendToEpisodeNotes(db, update.EpisodeID, update.NotesAddition)
		if g, ok := episodeMap[update.EpisodeID]; ok {
			for _, postID := range g.PostIDs {
				// Clear pending_questions optimistically so the router doesn't re-match
				// while the async re-summarization is in flight.
				_, _ = db.Exec(
					`UPDATE game_digest_weekly_post_notes SET pending_questions = '[]', is_complete = 1 WHERE post_id = ?`,
					postID,
				)
				var topicID int64
				var subforumID int
				if err := db.QueryRow(
					`SELECT t.id, t.subforum_id FROM posts p JOIN topics t ON t.id = p.topic_id WHERE p.id = ?`,
					postID,
				).Scan(&topicID, &subforumID); err == nil {
					go SummarizeEpisodePost(db, postID, topicID, subforumID)
				}
			}
		}
	}

	// Deliver reply.
	r, err := db.Exec(
		`INSERT INTO ai_chat_messages (user_id, role, content, date_created) VALUES (?, 'assistant', ?, NOW())`,
		userID, result.UserReply,
	)
	if err != nil {
		markFailed(db, taskID, userID, "failed to insert reply: "+err.Error())
		return
	}
	replyID, _ := r.LastInsertId()

	_, _ = db.Exec(`UPDATE ai_task_queue SET status = 'done', date_completed = NOW() WHERE id = ?`, taskID)

	msg := Entities.AIChatMessage{
		Id:          int(replyID),
		UserId:      userID,
		Role:        "assistant",
		Content:     result.UserReply,
		DateCreated: time.Now(),
	}

	wasSubscriber := notifySubscribersOnCompletion(db, taskID, userID, msg)
	if !wasSubscriber {
		Websockets.MainHub.SendNotification(userID, map[string]interface{}{
			"type": "ai_message",
			"data": msg,
		})
	}
	Events.Publish(db, Events.NotificationCreated, Events.NotificationEvent{
		UserID:  userID,
		Type:    "ai_message",
		Message: "The AI has sent you a message in the AI chat",
		Data:    Entities.NotificationAiMessage{},
	})
}

// appendToEpisodeNotes appends clarified context to the episode's additional_context field.
func appendToEpisodeNotes(db *sql.DB, episodeID int64, addition string) {
	var existing sql.NullString
	_ = db.QueryRow(`SELECT additional_context FROM game_digest_episode_notes WHERE episode_id = ?`, episodeID).Scan(&existing)

	var updated string
	if existing.Valid && existing.String != "" {
		updated = existing.String + "\n\n" + addition
	} else {
		updated = addition
	}

	_, _ = db.Exec(`
		INSERT INTO game_digest_episode_notes (episode_id, additional_context)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE additional_context = VALUES(additional_context)`,
		episodeID, updated,
	)
}

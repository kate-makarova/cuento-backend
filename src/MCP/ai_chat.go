package MCP

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// chatSystemInstruction builds the system prompt for AI chat tasks.
func chatSystemInstruction(userID int) string {
	return fmt.Sprintf(
		"You are a helpful assistant for a forum community. "+
			"The current user's ID is %d. Use this when calling tools that require a user_id.\n\n"+
			"SEARCH STRATEGY — follow these steps in order, each at most once:\n"+
			"Step 1. READ THE MESSAGE FOR EXPLICIT FILTERS before doing anything else. "+
			"Check whether the user has directly named a faction, character, episode, or subforum area. "+
			"If they named a faction → call get_factions once and resolve it to a faction_id; "+
			"apply that faction_id to all subsequent searches for characters and episodes. "+
			"If they named a specific character → target only the 'characters' bucket. "+
			"If they named a specific episode → target only the 'episodes' and 'game_posts' buckets. "+
			"If they named a subforum or area → note it as a context hint for picking relevant buckets. "+
			"If nothing is explicitly named, proceed without pre-set filters.\n"+
			"Step 2. Identify the core intent of the question.\n"+
			"Step 3. Extract 2–4 focused keyword phrases. Use specific names or short terms, not the full message.\n"+
			"Step 4. Call search_content once per keyword phrase, applying any filters determined in Step 1.\n"+
			"Step 5. FACTION DETECTION — only if no faction was identified in Step 1, and the question involves characters or episodes: "+
			"call get_factions once. Read through your Step 4 results and decide — "+
			"without any additional tool calls — whether the content clearly belongs to one of those factions. "+
			"If yes, call search_content once more per relevant bucket with that faction_id set. "+
			"If no faction is obvious, skip this step entirely.\n"+
			"Step 6. STOP searching. Do not loop back. "+
			"From all results collected so far, keep only entries that directly answer the question.\n"+
			"Step 7. Call cite_sources once with the 'id' values (as integers) of the specific entries you actually drew facts from. "+
			"Do NOT include entries you retrieved but did not use in your answer. If nothing was relevant, pass an empty array.\n"+
			"Step 8. Write your answer based on those entries. "+
			"If nothing relevant was found, say so and suggest different search terms.\n\n"+
			"HARD RULES:\n"+
			"1. Never answer community questions from memory — always search first.\n"+
			"2. Never invent or extrapolate character names, faction names, lore, usernames, or post contents.\n"+
			"3. Never include post IDs, topic IDs, or any raw technical identifiers in your response.\n"+
			"4. Always respond in the same language the user wrote their message in.\n"+
			"5. Never include raw links, post IDs, topic IDs, or URL-style references in your text response. "+
			"Links are rendered by the UI from cite_sources calls. You may naturally mention where something was found "+
			"(e.g. 'according to their character sheet', 'as noted in the episode entry') but do not output any IDs or URLs.",
		userID,
	)
}

// executeChatTask loads the task context, appends the user message to the session,
// and hands off to the router.
func executeChatTask(db *sql.DB, taskID, userID int) {
	var payloadStr string
	_ = db.QueryRow(`SELECT COALESCE(payload, '{}') FROM ai_task_queue WHERE id = ?`, taskID).Scan(&payloadStr)
	var taskPayload struct {
		SessionID string `json:"session_id"`
		MessageID int64  `json:"message_id"`
	}
	_ = json.Unmarshal([]byte(payloadStr), &taskPayload)

	userMsg, err := loadMessageByID(taskPayload.MessageID, db)
	if err != nil {
		markFailed(db, taskID, userID, "failed to load message: "+err.Error())
		return
	}

	sess := getOrCreateChatSession(taskPayload.SessionID)
	sess.mu.Lock()
	sess.messages = append(sess.messages, userMsg)
	history := make([]ChatMessage, len(sess.messages))
	copy(history, sess.messages)
	sess.mu.Unlock()

	routeChatMessage(db, taskID, userID, userMsg, history, sess)
}

// uniqueSources removes duplicate ChatSource entries, keeping the first
// occurrence of each (PostID, TopicID) pair.
func uniqueSources(sources []ChatSource) []ChatSource {
	type key struct {
		postID  int64
		topicID int64
	}
	seen := make(map[key]bool, len(sources))
	out := sources[:0]
	for _, s := range sources {
		var k key
		if s.PostID != nil {
			k.postID = *s.PostID
		}
		if s.TopicID != nil {
			k.topicID = *s.TopicID
		}
		if !seen[k] {
			seen[k] = true
			out = append(out, s)
		}
	}
	return out
}

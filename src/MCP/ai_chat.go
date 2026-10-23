package MCP

import (
	"database/sql"
	"encoding/json"
)

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

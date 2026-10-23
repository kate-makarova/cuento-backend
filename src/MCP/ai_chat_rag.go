package MCP

import (
	"context"
	"cuento-backend/src/Entities"
	"cuento-backend/src/Events"
	"cuento-backend/src/Services"
	"cuento-backend/src/Websockets"
	"database/sql"
	"encoding/json"
	"log"
	"time"
)

// executeRAGChat runs the standard RAG pipeline for an AI chat task.
func executeRAGChat(db *sql.DB, taskID, userID int, history []ChatMessage, sess *chatSession) {
	ctx := context.Background()

	client, err := getActivePool().ClientForMinSizeAndProtocol(Entities.AIModelSizeSmall, Entities.AIModelTypeText, Entities.AIModelProtocolOpenAI)
	if err != nil {
		markFailed(db, taskID, userID, "no AI model available: "+err.Error())
		return
	}

	var replyText string
	var sources []ChatSource

	err = withModelRetry(
		getActivePool(),
		Entities.AIModelSizeSmall, Entities.AIModelTypeText,
		client,
		func(c AIClient) error {
			var e error
			replyText, sources, e = NewAIAgent(c).Chat(ctx, history, chatSystemInstruction(userID))
			return e
		},
		func(modelID string) {
			_, _ = db.Exec(`UPDATE ai_task_queue SET retries = retries + 1 WHERE id = ?`, taskID)
			log.Printf("AI queue: task %d rate-limited on %s, trying next model", taskID, modelID)
		},
	)
	if err != nil {
		markFailed(db, taskID, userID, err.Error())
		pushError(userID, err.Error())
		notifyQueuePositions(db, taskID)
		return
	}

	replyText = Services.MarkdownToHTML(replyText)

	sess.mu.Lock()
	sess.messages = append(sess.messages, ChatMessage{Role: "assistant", Content: replyText})
	sess.mu.Unlock()

	sources = uniqueSources(sources)

	var sourcesJSON []byte
	if len(sources) > 0 {
		sourcesJSON, _ = json.Marshal(sources)
	}

	var replyID int64
	r, err := db.Exec(
		"INSERT INTO ai_chat_messages (user_id, role, content, sources, date_created) VALUES (?, 'assistant', ?, ?, NOW())",
		userID, replyText, sourcesJSON,
	)
	if err == nil {
		replyID, _ = r.LastInsertId()
	}

	_, _ = db.Exec(`UPDATE ai_task_queue SET status = 'done', date_completed = NOW() WHERE id = ?`, taskID)

	entitySources := make([]Entities.AISource, len(sources))
	for i, s := range sources {
		entitySources[i] = Entities.AISource{
			PostID:    s.PostID,
			TopicID:   s.TopicID,
			TopicName: s.TopicName,
			TopicType: s.TopicType,
		}
	}
	msg := Entities.AIChatMessage{
		Id:          int(replyID),
		UserId:      userID,
		Role:        "assistant",
		Content:     replyText,
		Sources:     entitySources,
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

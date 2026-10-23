package MCP

import "database/sql"

// routeChatMessage decides which AI routine handles this message.
// Add new cases here as additional routines are introduced.
func routeChatMessage(db *sql.DB, taskID, userID int, userMsg ChatMessage, history []ChatMessage, sess *chatSession) {
	pendingDigest := loadPendingDigestRequests(db, userID)

	switch {
	case len(pendingDigest) > 0:
		executeDigestAnswerResolver(db, taskID, userID, userMsg.Content, pendingDigest)
	default:
		executeRAGChat(db, taskID, userID, history, sess)
	}
}

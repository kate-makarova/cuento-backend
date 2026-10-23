package MCP

import (
	"database/sql"
	"encoding/json"
	"log"
)

// RunGameDigestIfDue checks every active GameDigest implementation and enqueues
// a task for any that haven't run since the start of their configured period.
func RunGameDigestIfDue(db *sql.DB) {
	rows, err := db.Query(`
		SELECT ai.id, ai.config
		FROM ai_agent_implementation ai
		JOIN ai_agents a ON a.id = ai.agent_id
		WHERE a.handler = 'GameDigest' AND ai.is_active = 1`)
	if err != nil {
		log.Printf("game_digest scheduler: failed to query implementations: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var implID int
		var configStr sql.NullString
		if err := rows.Scan(&implID, &configStr); err != nil {
			continue
		}

		var cfg struct {
			Period string `json:"period"`
		}
		cfg.Period = "weekly"
		if configStr.Valid {
			_ = json.Unmarshal([]byte(configStr.String), &cfg)
		}

		if isGameDigestDue(db, implID, cfg.Period) {
			enqueueGameDigest(db, implID)
		}
	}
}

// isGameDigestDue returns true if no completed task exists since the start of the current period.
func isGameDigestDue(db *sql.DB, implID int, period string) bool {
	periodStart := periodToTime(period)
	var lastCompleted sql.NullTime
	_ = db.QueryRow(`
		SELECT MAX(date_completed)
		FROM ai_task_queue
		WHERE type = 'GameDigest'
		  AND status = 'done'
		  AND JSON_EXTRACT(payload, '$.implementation_id') = ?`,
		implID,
	).Scan(&lastCompleted)

	return !lastCompleted.Valid || lastCompleted.Time.Before(periodStart)
}

func enqueueGameDigest(db *sql.DB, implID int) {
	payload, _ := json.Marshal(map[string]int{"implementation_id": implID})
	if _, err := db.Exec(
		`INSERT INTO ai_task_queue (type, payload, status, date_created) VALUES ('GameDigest', ?, 'pending', NOW())`,
		string(payload),
	); err != nil {
		log.Printf("game_digest scheduler: failed to enqueue for implementation %d: %v", implID, err)
		return
	}
	NotifyWorker()
}

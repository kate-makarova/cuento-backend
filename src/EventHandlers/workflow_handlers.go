package EventHandlers

import (
	"cuento-backend/src/Events"
	"cuento-backend/src/Services"
	"database/sql"
	"encoding/json"
	"fmt"
)

type WorkflowHandlerFunc func(db *sql.DB, data Events.EventData, config json.RawMessage)

var workflowHandlers = map[string]WorkflowHandlerFunc{
	"MoveTopic": moveTopicHandler,
}

func moveTopicHandler(db *sql.DB, data Events.EventData, config json.RawMessage) {
	event, ok := data.(Events.TopicFullEvent)
	if !ok {
		return
	}

	var cfg struct {
		TargetSubforumID int `json:"target_subforum_id"`
	}
	if err := json.Unmarshal(config, &cfg); err != nil || cfg.TargetSubforumID == 0 {
		fmt.Printf("MoveTopic workflow: invalid config: %v\n", err)
		return
	}

	if err := Services.MoveTopics(db, []int{int(event.TopicID)}, cfg.TargetSubforumID); err != nil {
		fmt.Printf("MoveTopic workflow: failed to move topic %d: %v\n", event.TopicID, err)
	}
}

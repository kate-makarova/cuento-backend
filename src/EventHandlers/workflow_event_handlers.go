package EventHandlers

import (
	"cuento-backend/src/Events"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func RegisterWorkflowEventHandlers() {
	Events.Subscribe(Events.TopicFull, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.TopicFullEvent)
		if !ok {
			return
		}

		rows, err := db.Query(
			"SELECT subforum_ids, handler_function, config FROM workflows WHERE event_name = ?",
			string(Events.TopicFull),
		)
		if err != nil {
			fmt.Printf("WorkflowHandler: failed to query workflows: %v\n", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var subforumIDs string
			var handlerName string
			var config json.RawMessage
			if err := rows.Scan(&subforumIDs, &handlerName, &config); err != nil {
				continue
			}
			if !subforumInList(event.SubforumID, subforumIDs) {
				continue
			}
			handler, ok := workflowHandlers[handlerName]
			if !ok {
				fmt.Printf("WorkflowHandler: unknown handler function %q\n", handlerName)
				continue
			}
			handler(db, data, config)
		}
	})
}

func subforumInList(subforumID int, list string) bool {
	for _, part := range strings.Split(list, ",") {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil && id == subforumID {
			return true
		}
	}
	return false
}

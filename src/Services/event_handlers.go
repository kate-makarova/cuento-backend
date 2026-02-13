package Services

import (
	"cuento-backend/src/Events"
	"database/sql"
	"fmt"
)

func RegisterEventHandlers(db *sql.DB) {
	// Subscriber 1: Update Global Stats
	Events.Subscribe(Events.TopicCreated, func(db *sql.DB, data Events.EventData) {
		// We don't use the event data here, but we cast it to ensure it's the right event
		_, ok := data.(Events.TopicCreatedEvent)
		if !ok {
			return
		}

		_, err := db.Exec("UPDATE global_stats SET stat_value = stat_value + 1 WHERE stat_name = 'total_post_number'")
		if err != nil {
			fmt.Printf("Error updating global post stats: %v\n", err)
		}
		_, err = db.Exec("UPDATE global_stats SET stat_value = stat_value + 1 WHERE stat_name = 'total_topic_number'")
		if err != nil {
			fmt.Printf("Error updating global topic stats: %v\n", err)
		}
	})

	// Subscriber 2: Update Subforum Stats
	Events.Subscribe(Events.TopicCreated, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.TopicCreatedEvent)
		if !ok {
			return
		}

		_, err := db.Exec("UPDATE subforums SET topic_number = topic_number + 1, post_number = post_number + 1, last_post_topic_id = ?, last_post_topic_name = ?, last_post_id = ?, date_last_post = NOW(), last_post_author_user_name = ? WHERE id = ?",
			event.TopicID, event.Title, event.PostID, event.Username, event.SubforumID)
		if err != nil {
			fmt.Printf("Error updating subforum stats: %v\n", err)
		}
	})
}

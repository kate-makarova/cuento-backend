package EventHandlers

import (
	"cuento-backend/src/Events"
	"database/sql"
	"fmt"
	"strings"
)

func RegisterUserEventHandlers() {
	// Subscriber: Update date_last_visit when user becomes active or inactive
	Events.Subscribe(Events.UserActivityChanged, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.UserActivityChangedEvent)
		if !ok {
			return
		}
		_, err := db.Exec("UPDATE users SET date_last_visit = NOW() WHERE id = ?", event.UserID)
		if err != nil {
			fmt.Printf("Error updating date_last_visit for user %d: %v\n", event.UserID, err)
		}
	})

	// Subscriber 15: Update Global Stats on User Registered
	Events.Subscribe(Events.UserRegistered, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.UserRegisteredEvent)
		if !ok {
			return
		}

		// 1. Update total user number
		_, err := db.Exec("UPDATE global_stats SET stat_value = stat_value + 1 WHERE stat_name = 'total_user_number'")
		if err != nil {
			fmt.Printf("Error updating global user stats: %v\n", err)
		}

		// 2. Update last user
		_, err = db.Exec("UPDATE global_stats SET stat_value = ?, stat_secondary = ? WHERE stat_name = 'last_user'", event.UserID, event.Username)
		if err != nil {
			fmt.Printf("Error updating last user global stat: %v\n", err)
		}
	})

	// Subscriber: Update counters when a user account is wiped
	Events.Subscribe(Events.UserWiped, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.UserWipedEvent)
		if !ok {
			return
		}

		// Decrement global user count
		_, _ = db.Exec("UPDATE global_stats SET stat_value = GREATEST(stat_value - 1, 0) WHERE stat_name = 'total_user_number'")

		// Decrement global post count for each deleted general post
		if n := len(event.DeletedGeneralPostIDs); n > 0 {
			_, _ = db.Exec(
				"UPDATE global_stats SET stat_value = GREATEST(stat_value - ?, 0) WHERE stat_name = 'total_post_number'",
				n,
			)
		}

		// Recalculate post_number + last-post metadata for every affected topic
		for _, topicID := range event.AffectedTopicIDs {
			_, _ = db.Exec(`
				UPDATE topics SET
					post_number              = (SELECT COUNT(*) FROM posts WHERE topic_id = ? AND COALESCE(is_deleted, 0) != 1),
					date_last_post           = (SELECT MAX(date_created) FROM posts WHERE topic_id = ? AND COALESCE(is_deleted, 0) != 1),
					last_post_author_user_id = (SELECT author_user_id FROM posts WHERE topic_id = ? AND COALESCE(is_deleted, 0) != 1 ORDER BY date_created DESC LIMIT 1)
				WHERE id = ?`,
				topicID, topicID, topicID, topicID)
		}

		// Refresh subforum stats for every affected subforum
		for _, subforumID := range event.AffectedSubforumIDs {
			refreshSubforumStats(db, subforumID)
			Events.Publish(db, Events.SubforumUpdated, Events.SubforumUpdatedEvent{SubforumID: subforumID})
		}

		if len(event.AffectedSubforumIDs) > 0 {
			ids := make([]string, len(event.AffectedSubforumIDs))
			for i, id := range event.AffectedSubforumIDs {
				ids[i] = fmt.Sprintf("%d", id)
			}
			fmt.Printf("UserWiped: refreshed subforum stats for subforums [%s]\n", strings.Join(ids, ", "))
		}
	})
}

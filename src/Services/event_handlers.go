package Services

import (
	"cuento-backend/src/Events"
	"cuento-backend/src/Websockets"
	"database/sql"
	"fmt"
	"strconv"
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

	// Subscriber 3: Send Live Notifications
	Events.Subscribe(Events.NotificationCreated, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.NotificationEvent)
		if !ok {
			return
		}
		Websockets.MainHub.SendNotification(event.UserID, event)
	})

	// Subscriber 4: Notify Topic Viewers
	Events.Subscribe(Events.UserReadingTopic, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.UserReadingTopicEvent)
		if !ok {
			return
		}

		// Get all users currently reading this topic
		users := ActivityStorage.GetUsersOnPage("topic", event.TopicID)

		// Construct the notification message
		// We want to send the list of users to everyone on the page
		type Viewer struct {
			UserID   int    `json:"user_id"`
			Username string `json:"username"`
		}
		var viewerList []Viewer
		for _, u := range users {
			viewerList = append(viewerList, Viewer{
				UserID:   u.UserID,
				Username: u.Username,
			})
		}

		notification := map[string]interface{}{
			"type": "topic_viewers_update",
			"data": viewerList,
		}

		// Send to each user on the page
		for _, u := range users {
			Websockets.MainHub.SendNotification(u.UserID, notification)
		}
	})

	// Subscriber 5: Notify New Post in Topic
	Events.Subscribe(Events.PostCreated, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.PostCreatedEvent)
		if !ok {
			return
		}

		// Get all users currently reading this topic
		topicIDStr := strconv.FormatInt(event.TopicID, 10)
		users := ActivityStorage.GetUsersOnPage("topic", topicIDStr)

		notification := map[string]interface{}{
			"type": "new_post",
			"data": event.Post,
		}

		// Send to each user on the page
		for _, u := range users {
			Websockets.MainHub.SendNotification(u.UserID, notification)
		}
	})

	// Subscriber 6: Update Stats on Post Created
	Events.Subscribe(Events.PostCreated, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.PostCreatedEvent)
		if !ok {
			return
		}

		// 1. Update Global Stats
		_, err := db.Exec("UPDATE global_stats SET stat_value = stat_value + 1 WHERE stat_name = 'total_post_number'")
		if err != nil {
			fmt.Printf("Error updating global post stats: %v\n", err)
		}

		// 2. Update Topic Stats
		_, err = db.Exec("UPDATE topics SET post_number = post_number + 1, date_last_post = NOW(), last_post_author_user_id = ? WHERE id = ?",
			event.Post.AuthorUserId, event.TopicID)
		if err != nil {
			fmt.Printf("Error updating topic stats: %v\n", err)
		}

		// 3. Update Subforum Stats
		// Need to fetch username and topic title
		var username string
		err = db.QueryRow("SELECT username FROM users WHERE id = ?", event.Post.AuthorUserId).Scan(&username)
		if err != nil {
			fmt.Printf("Error fetching username for stats: %v\n", err)
			return
		}

		var topicTitle string
		err = db.QueryRow("SELECT name FROM topics WHERE id = ?", event.TopicID).Scan(&topicTitle)
		if err != nil {
			fmt.Printf("Error fetching topic title for stats: %v\n", err)
			return
		}

		_, err = db.Exec("UPDATE subforums SET post_number = post_number + 1, last_post_topic_id = ?, last_post_topic_name = ?, last_post_id = ?, date_last_post = NOW(), last_post_author_user_name = ? WHERE id = ?",
			event.TopicID, topicTitle, event.Post.Id, username, event.SubforumID)
		if err != nil {
			fmt.Printf("Error updating subforum stats: %v\n", err)
		}
	})
}

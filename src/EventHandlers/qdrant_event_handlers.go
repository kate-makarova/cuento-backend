package EventHandlers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Events"
	"cuento-backend/src/Services"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

func RegisterQdrantEventHandlers() {
	Events.Subscribe(Events.PostCreated, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.PostCreatedEvent)
		if !ok || event.Type == "post_updated" {
			return
		}
		if !Services.QdrantAvailable() {
			return
		}

		var topicType Entities.TopicType
		if err := db.QueryRow("SELECT type FROM topics WHERE id = ?", event.TopicID).Scan(&topicType); err != nil {
			return
		}

		var bucket string
		switch topicType {
		case Entities.EpisodeTopic:
			bucket = Services.SonicBucketGamePosts
		case Entities.GeneralTopic:
			bucket = Services.SonicBucketGeneralPosts
		case Entities.LoreTopic:
			bucket = Services.SonicBucketLorePosts
		default:
			return
		}

		objectID := strconv.Itoa(event.Post.Id)
		if err := Services.QdrantPush(bucket, objectID, event.Post.Content); err != nil {
			fmt.Printf("Error pushing post %s to Qdrant: %v\n", objectID, err)
		}
	})

	Events.Subscribe(Events.CharacterCreated, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.CharacterCreatedEvent)
		if !ok {
			return
		}
		if !Services.QdrantAvailable() {
			return
		}
		if err := Services.QdrantPushFlattenedEntity(Services.SonicBucketCharacters, "character", event.CharacterID, db); err != nil {
			fmt.Printf("Error pushing character %d to Qdrant: %v\n", event.CharacterID, err)
		}
	})

	Events.Subscribe(Events.EpisodeCreated, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.EpisodeCreatedEvent)
		if !ok {
			return
		}
		if !Services.QdrantAvailable() {
			return
		}
		if err := Services.QdrantPushFlattenedEntity(Services.SonicBucketEpisodes, "episode", event.EpisodeID, db); err != nil {
			fmt.Printf("Error pushing episode %d to Qdrant: %v\n", event.EpisodeID, err)
		}
	})

	Events.Subscribe(Events.WantedCharacterCreated, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.WantedCharacterCreatedEvent)
		if !ok {
			return
		}
		if !Services.QdrantAvailable() {
			return
		}
		if err := Services.QdrantPushFlattenedEntity(Services.SonicBucketWantedPosts, "wanted_character", event.WantedCharacterID, db); err != nil {
			fmt.Printf("Error pushing wanted character %d to Qdrant: %v\n", event.WantedCharacterID, err)
		}
	})

	Events.Subscribe(Events.PostCreated, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.PostCreatedEvent)
		if !ok || event.Type != "post_deleted" {
			return
		}
		if !Services.QdrantAvailable() {
			return
		}

		var topicType Entities.TopicType
		if err := db.QueryRow("SELECT type FROM topics WHERE id = ?", event.TopicID).Scan(&topicType); err != nil {
			return
		}

		var bucket string
		switch topicType {
		case Entities.EpisodeTopic:
			bucket = Services.SonicBucketGamePosts
		case Entities.GeneralTopic:
			bucket = Services.SonicBucketGeneralPosts
		case Entities.LoreTopic:
			bucket = Services.SonicBucketLorePosts
		default:
			return
		}

		if err := Services.QdrantDelete(bucket, strconv.Itoa(event.Post.Id)); err != nil {
			fmt.Printf("Error deleting post %d from Qdrant: %v\n", event.Post.Id, err)
		}
	})

	Events.Subscribe(Events.TopicsDeleted, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.TopicsDeletedEvent)
		if !ok || len(event.TopicIDs) == 0 {
			return
		}
		if !Services.QdrantAvailable() {
			return
		}

		placeholders := strings.Repeat("?,", len(event.TopicIDs)-1) + "?"
		args := make([]interface{}, len(event.TopicIDs))
		for i, id := range event.TopicIDs {
			args[i] = id
		}

		postIDs := map[string][]string{
			Services.SonicBucketGamePosts:    {},
			Services.SonicBucketGeneralPosts: {},
			Services.SonicBucketLorePosts:    {},
		}
		postRows, err := db.Query(
			fmt.Sprintf("SELECT p.id, t.type FROM posts p JOIN topics t ON p.topic_id = t.id WHERE t.id IN (%s)", placeholders),
			args...,
		)
		if err == nil {
			defer postRows.Close()
			for postRows.Next() {
				var postID int64
				var topicType Entities.TopicType
				if postRows.Scan(&postID, &topicType) != nil {
					continue
				}
				var bucket string
				switch topicType {
				case Entities.EpisodeTopic:
					bucket = Services.SonicBucketGamePosts
				case Entities.GeneralTopic:
					bucket = Services.SonicBucketGeneralPosts
				case Entities.LoreTopic:
					bucket = Services.SonicBucketLorePosts
				default:
					continue
				}
				postIDs[bucket] = append(postIDs[bucket], strconv.FormatInt(postID, 10))
			}
		}
		for bucket, ids := range postIDs {
			if err := Services.QdrantDeleteBatch(bucket, ids); err != nil {
				fmt.Printf("Error deleting posts from Qdrant bucket %s on topic delete: %v\n", bucket, err)
			}
		}

		charRows, err := db.Query(
			fmt.Sprintf("SELECT id FROM character_base WHERE topic_id IN (%s)", placeholders),
			args...,
		)
		if err == nil {
			defer charRows.Close()
			for charRows.Next() {
				var charID int64
				if charRows.Scan(&charID) == nil {
					_ = Services.QdrantDelete(Services.SonicBucketCharacters, strconv.FormatInt(charID, 10))
				}
			}
		}

		wantedRows, err := db.Query(
			fmt.Sprintf("SELECT id FROM wanted_character_base WHERE topic_id IN (%s)", placeholders),
			args...,
		)
		if err == nil {
			defer wantedRows.Close()
			for wantedRows.Next() {
				var wcID int64
				if wantedRows.Scan(&wcID) == nil {
					_ = Services.QdrantDelete(Services.SonicBucketWantedPosts, strconv.FormatInt(wcID, 10))
				}
			}
		}
	})

	Events.Subscribe(Events.UserWiped, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.UserWipedEvent)
		if !ok || len(event.DeletedGeneralPostIDs) == 0 {
			return
		}
		if !Services.QdrantAvailable() {
			return
		}

		ids := make([]string, len(event.DeletedGeneralPostIDs))
		for i, id := range event.DeletedGeneralPostIDs {
			ids[i] = strconv.Itoa(id)
		}
		if err := Services.QdrantDeleteBatch(Services.SonicBucketGeneralPosts, ids); err != nil {
			fmt.Printf("Error deleting general posts from Qdrant on user wipe: %v\n", err)
		}
	})

	Events.Subscribe(Events.EpisodeTopicsDeleted, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.EpisodeTopicsDeletedEvent)
		if !ok || len(event.EpisodeIDs) == 0 {
			return
		}
		if !Services.QdrantAvailable() {
			return
		}

		ids := make([]string, len(event.EpisodeIDs))
		for i, id := range event.EpisodeIDs {
			ids[i] = strconv.Itoa(id)
		}
		if err := Services.QdrantDeleteBatch(Services.SonicBucketEpisodes, ids); err != nil {
			fmt.Printf("Error deleting episodes from Qdrant: %v\n", err)
		}
	})
}

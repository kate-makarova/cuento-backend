package EventHandlers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Events"
	"cuento-backend/src/Services"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/expectedsh/go-sonic/sonic"
)

func RegisterSonicEventHandlers() {
	Events.Subscribe(Events.PostCreated, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.PostCreatedEvent)
		if !ok || event.Type == "post_updated" {
			return
		}
		if !Services.SonicAvailable() {
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
		if err := Services.SonicPush(Services.SonicCollection, bucket, objectID, event.Post.Content, sonic.LangAutoDetect); err != nil {
			fmt.Printf("Error pushing post %s to Sonic: %v\n", objectID, err)
		}
	})

	Events.Subscribe(Events.CharacterCreated, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.CharacterCreatedEvent)
		if !ok {
			return
		}
		if !Services.SonicAvailable() {
			return
		}
		if err := Services.SonicPushFlattenedEntity(Services.SonicBucketCharacters, "character", event.CharacterID, db); err != nil {
			fmt.Printf("Error pushing character %d to Sonic: %v\n", event.CharacterID, err)
		}
	})

	Events.Subscribe(Events.EpisodeCreated, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.EpisodeCreatedEvent)
		if !ok {
			return
		}
		if !Services.SonicAvailable() {
			return
		}
		if err := Services.SonicPushFlattenedEntity(Services.SonicBucketEpisodes, "episode", event.EpisodeID, db); err != nil {
			fmt.Printf("Error pushing episode %d to Sonic: %v\n", event.EpisodeID, err)
		}
	})

	Events.Subscribe(Events.WantedCharacterCreated, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.WantedCharacterCreatedEvent)
		if !ok {
			return
		}
		if !Services.SonicAvailable() {
			return
		}
		if err := Services.SonicPushFlattenedEntity(Services.SonicBucketWantedPosts, "wanted_character", event.WantedCharacterID, db); err != nil {
			fmt.Printf("Error pushing wanted character %d to Sonic: %v\n", event.WantedCharacterID, err)
		}
	})
}

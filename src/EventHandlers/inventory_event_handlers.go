package EventHandlers

import (
	"cuento-backend/src/Events"
	"cuento-backend/src/Services"
	"database/sql"
	"log"
)

func RegisterInventoryEventHandlers() {
	// Each new in-character post in a scene with a locked inventory gets its own copy of that inventory.
	Events.Subscribe(Events.PostCreated, func(db *sql.DB, data Events.EventData) {
		event, ok := data.(Events.PostCreatedEvent)
		if !ok || (event.Type != "" && event.Type != "post_created") {
			return
		}
		if err := Services.SnapshotPostInventory(db, int64(event.Post.Id)); err != nil {
			log.Printf("inventory: %v", err)
		}
	})
}

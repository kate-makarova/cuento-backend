package Events

import (
	"database/sql"
	"sync"
)

type EventType string

const (
	TopicCreated EventType = "TopicCreated"
)

type EventData interface{}

type TopicCreatedEvent struct {
	TopicID    int64
	SubforumID int
	Title      string
	PostID     int64
	UserID     int
	Username   string
}

type EventHandler func(db *sql.DB, data EventData)

var (
	subscribers = make(map[EventType][]EventHandler)
	mu          sync.RWMutex
)

func Subscribe(eventType EventType, handler EventHandler) {
	mu.Lock()
	defer mu.Unlock()
	subscribers[eventType] = append(subscribers[eventType], handler)
}

func Publish(db *sql.DB, eventType EventType, data EventData) {
	mu.RLock()
	defer mu.RUnlock()
	if handlers, found := subscribers[eventType]; found {
		for _, handler := range handlers {
			// Run handlers in a goroutine to avoid blocking the main request
			go handler(db, data)
		}
	}
}

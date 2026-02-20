package Events

import (
	"cuento-backend/src/Entities"
	"database/sql"
	"sync"
)

type EventType string

const (
	TopicCreated        EventType = "TopicCreated"
	PostCreated         EventType = "PostCreated"
	NotificationCreated EventType = "NotificationCreated"
)

type EventData interface{}

type TopicCreatedEvent struct {
	Type       string `json:"type"`
	TopicID    int64
	SubforumID int
	Title      string
	PostID     int64
	UserID     int
	Username   string
}

type PostCreatedEvent struct {
	Type       string        `json:"type"`
	TopicID    int64         `json:"topic_id"`
	SubforumID int           `json:"subforum_id"`
	Post       Entities.Post `json:"post"`
}

type NotificationEvent struct {
	UserID  int         `json:"user_id"`
	Type    string      `json:"type"` // e.g., "info", "success", "error"
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
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

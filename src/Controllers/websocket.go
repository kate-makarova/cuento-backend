package Controllers

import (
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"cuento-backend/src/Websockets"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins
	},
}

func HandleWebSocket(c *gin.Context, db *sql.DB) {
	userID := Services.GetUserIdFromContext(c)
	if userID == 0 {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusUnauthorized, Message: "Unauthorized"})
		c.Abort()
		return
	}

	// Fetch username for activity tracking
	var username string
	err := db.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&username)
	if err != nil {
		// If user not found or db error, maybe abort?
		// For now, let's proceed but maybe log it?
		// Or just return error.
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get user details"})
		c.Abort()
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &Websockets.Client{
		Hub:    Websockets.MainHub,
		Conn:   conn,
		Send:   make(chan interface{}, 256),
		UserID: userID,
	}

	Websockets.MainHub.Register(client)
	Services.ActivityStorage.AddUser(userID, username)

	// Read loop to keep connection alive and detect disconnects
	go func() {
		defer func() {
			Services.ActivityStorage.RemoveUser(userID)
			Websockets.MainHub.Unregister(client)
			conn.Close()
		}()

		// Set up Ping/Pong handlers to keep connection alive
		conn.SetReadLimit(512)
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error { conn.SetReadDeadline(time.Now().Add(60 * time.Second)); return nil })

		for {
			_, p, err := conn.ReadMessage()
			if err != nil {
				break
			}

			var msg struct {
				Type     string `json:"type"`
				PageType string `json:"page_type"`
				PageId   string `json:"page_id"`
			}
			if err := json.Unmarshal(p, &msg); err == nil {
				if msg.Type == "page_change" {
					Services.ActivityStorage.UpdateUserLocation(userID, msg.PageType, msg.PageId)
				}
			}
		}
	}()
}

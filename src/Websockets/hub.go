package Websockets

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	Hub    *Hub
	Conn   *websocket.Conn
	Send   chan interface{}
	UserID int
}

func (c *Client) writePump() {
	defer func() {
		// When the write pump exits, it's crucial to unregister the client
		// to signal that this client is gone. The readPump's defer will
		// handle the actual connection closing.
		c.Hub.unregister <- c
	}()
	ticker := time.NewTicker(54 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// The Hub closed the channel, which means the client is being disconnected.
				// Send a close message to the client for a clean shutdown.
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// If writing to the connection fails, the client is considered disconnected.
			// We return from the function, which will trigger the deferred cleanup.
			if err := c.Conn.WriteJSON(message); err != nil {
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

type Hub struct {
	clients    map[int]map[*Client]bool
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

var MainHub = NewHub()

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[int]map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.clients[client.UserID] == nil {
				h.clients[client.UserID] = make(map[*Client]bool)
			}
			h.clients[client.UserID][client] = true
			h.mu.Unlock()
			go client.writePump()

		case client := <-h.unregister:
			h.mu.Lock()
			if userClients, ok := h.clients[client.UserID]; ok {
				if _, ok := userClients[client]; ok {
					delete(userClients, client)
					close(client.Send)
					if len(userClients) == 0 {
						delete(h.clients, client.UserID)
					}
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) Register(client *Client) {
	h.register <- client
}

func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

func (h *Hub) SendNotification(userID int, message interface{}) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if userClients, ok := h.clients[userID]; ok {
		for client := range userClients {
			// Use a closure and a recover to prevent a panic from a "send on closed channel"
			// race condition. This can happen if a client disconnects and is unregistered
			// at the same time a message is being sent to them.
			func() {
				defer func() {
					if r := recover(); r != nil {
						// The panic is recovered. The client is already being cleaned up,
						// so we don't need to do anything else.
					}
				}()
				select {
				case client.Send <- message:
					// Message sent successfully.
				default:
					// Client's send buffer is full. Drop the message.
				}
			}()
		}
	}
}

func (h *Hub) Broadcast(message interface{}) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, userClients := range h.clients {
		for client := range userClients {
			func() {
				defer func() {
					if r := recover(); r != nil {
						// The panic is recovered. The client is already being cleaned up.
					}
				}()
				select {
				case client.Send <- message:
					// Message sent successfully.
				default:
					// Client's send buffer is full. Drop the message.
				}
			}()
		}
	}
}

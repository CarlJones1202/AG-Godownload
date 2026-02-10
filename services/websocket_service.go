package services

import (
	"gallery_api/logger"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

type Client struct {
	conn *websocket.Conn
	send chan interface{}
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan interface{}
	register   chan *Client
	unregister chan *Client
	mu         sync.Mutex
}

var globalHub = &Hub{
	clients:    make(map[*Client]bool),
	broadcast:  make(chan interface{}),
	register:   make(chan *Client),
	unregister: make(chan *Client),
}

func StartWebSocketHub() {
	go func() {
		for {
			select {
			case client := <-globalHub.register:
				globalHub.mu.Lock()
				globalHub.clients[client] = true
				globalHub.mu.Unlock()
				logger.Debug("WebSocket client registered")

				// Send initial status
				client.send <- GetGlobalDownloadStatus()

			case client := <-globalHub.unregister:
				globalHub.mu.Lock()
				if _, ok := globalHub.clients[client]; ok {
					delete(globalHub.clients, client)
					close(client.send)
				}
				globalHub.mu.Unlock()
				logger.Debug("WebSocket client unregistered")

			case message := <-globalHub.broadcast:
				globalHub.mu.Lock()
				for client := range globalHub.clients {
					select {
					case client.send <- message:
					default:
						close(client.send)
						delete(globalHub.clients, client)
					}
				}
				globalHub.mu.Unlock()
			}
		}
	}()
}

func HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("Failed to upgrade WebSocket:", err)
		return
	}

	client := &Client{conn: conn, send: make(chan interface{}, 256)}
	globalHub.register <- client

	// Read loop (to keep connection alive and detect disconnects)
	go func() {
		defer func() {
			globalHub.unregister <- client
			conn.Close()
		}()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()

	// Write loop
	go func() {
		for message := range client.send {
			err := conn.WriteJSON(message)
			if err != nil {
				logger.Error("WebSocket write error:", err)
				break
			}
		}
	}()
}

func BroadcastStatus(status interface{}) {
	// Use a non-blocking send to the hub
	select {
	case globalHub.broadcast <- status:
	default:
		// Drop message if hub is busy
	}
}

// internal/ws/client.go
package ws

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// writeWait is the maximum time allowed to write a single message to the peer.
	// If a write takes longer than this, the connection is considered dead.
	writeWait = 10 * time.Second

	// pongWait is how long ReadPump waits for any activity before declaring the
	// connection dead. Includes pong replies to our pings.
	pongWait = 60 * time.Second

	// pingPeriod controls how often WritePump sends a ping frame to the client.
	// Must be less than pongWait so the pong has time to arrive before the
	// read deadline fires.
	pingPeriod = 54 * time.Second

	// maxMessageSize is the largest binary message (Yjs update) we will accept.
	// 512 KB is generous — real Yjs updates are typically a few hundred bytes.
	maxMessageSize = 512 * 1024
)

// Client represents one WebSocket connection — one browser tab.
// It is owned by the Room it belongs to.
//
// Communication rules:
//   - ReadPump is the only goroutine that reads from conn.
//   - WritePump is the only goroutine that writes to conn.
//   - Everything else communicates with this client by sending to client.send.
//   - When the Room closes client.send, WritePump sends a close frame and exits.
type Client struct {
	// The raw WebSocket connection. Never touched outside ReadPump / WritePump.
	conn *websocket.Conn

	// The Room this client belongs to. ReadPump sends incoming messages here.
	room *Room

	// Buffered channel of outbound messages. The Room writes here; WritePump drains it.
	// Buffered at 256 so a momentarily slow client doesn't block the Room's event loop.
	send chan []byte

	// Identity fields set once at connection time and never changed.
	userID     string
	color      string // assigned cursor colour for this editing session
	documentID string
}

// NewClient constructs a Client. Called by the WebSocket handler after upgrade.
func NewClient(conn *websocket.Conn, room *Room, userID, color, documentID string) *Client {
	return &Client{
		conn:       conn,
		room:       room,
		send:       make(chan []byte, 256),
		userID:     userID,
		color:      color,
		documentID: documentID,
	}
}

// ReadPump pumps messages from the WebSocket connection into the Room.
//
// This goroutine runs for the entire lifetime of the connection.
// When it exits (error, close frame, deadline exceeded), it unregisters
// the client from the Room — which eventually closes client.send —
// which causes WritePump to exit too.
//
// Call this in a goroutine: go client.ReadPump()
func (c *Client) ReadPump() {
	defer func() {
		// Unregister triggers the Room to close c.send, which exits WritePump.
		c.room.unregister <- c
	}()

	c.conn.SetReadLimit(maxMessageSize)

	// Deadline: if no message (including pong) arrives within pongWait, ReadMessage
	// returns an error and this goroutine exits.
	c.conn.SetReadDeadline(time.Now().Add(pongWait))

	// Every time a pong arrives, push the deadline forward another pongWait window.
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		// ReadMessage blocks until a message arrives, an error occurs,
		// or the read deadline fires.
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			// websocket.IsUnexpectedCloseError filters out normal close events
			// so we only log genuine unexpected disconnections.
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseNormalClosure,
				websocket.CloseNoStatusReceived,
			) {
				log.Printf("ws: unexpected close for user %s doc %s: %v", c.userID, c.documentID, err)
			}
			return // triggers defer → unregister
		}

		// Guard: every document message must have at least the type byte.
		if len(message) == 0 {
			continue
		}

		// Route by the first byte — the Yjs message type.
		switch message[0] {
		case MsgSync:
			// Yjs update or sync handshake step — must be persisted, then broadcast.
			c.room.broadcast <- &RoomMessage{sender: c, data: message}

		case MsgAwareness:
			// Cursor / presence update — broadcast only, never persisted.
			c.room.awareness <- &RoomMessage{sender: c, data: message}

		default:
			// Unknown type — log and discard. Never crash on bad input.
			log.Printf("ws: unknown message type 0x%02x from user %s", message[0], c.userID)
		}
	}
}

// WritePump pumps messages from client.send to the WebSocket connection.
//
// This goroutine also owns the ping ticker — it sends a ping every pingPeriod.
// ReadPump's pong handler resets the read deadline on receipt.
//
// Call this in a goroutine: go client.WritePump()
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)

	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if !ok {
				// The Room closed this channel — send a clean close frame and exit.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.BinaryMessage, message); err != nil {
				// Write failed — connection is dead. Exit so the conn gets closed.
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
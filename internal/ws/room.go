// internal/ws/room.go
package ws

import (
	"context"
	"log"

	"github.com/saqlainsyb/docflow-core/internal/services"
)

// RoomMessage wraps a raw WebSocket message with a pointer to the sender.
// The Room needs the sender so it can broadcast to everyone except them.
type RoomMessage struct {
	sender *Client
	data   []byte
}

// Room manages all clients connected to a single document.
//
// Lifecycle:
//   - Created lazily by the Hub when the first client connects.
//   - Destroyed (goroutine exits) when the last client disconnects, or when
//     Hub.Shutdown() calls Close().
//   - The Hub is notified via room.done closing.
//
// Concurrency rule:
//   - Run() is the ONLY goroutine that reads or writes room.clients.
//   - All other goroutines communicate with Run() through channels only.
type Room struct {
	documentID string

	// clients is the set of currently connected clients.
	// map[*Client]bool — value is always true, the map is used as a set.
	// ONLY touched inside Run().
	clients map[*Client]bool

	// broadcast receives Yjs sync messages (real document edits).
	// Run() persists them to the database, then forwards to all other clients.
	broadcast chan *RoomMessage

	// awareness receives cursor / presence messages.
	// Run() forwards to all other clients. Never persisted.
	awareness chan *RoomMessage

	// register is sent by the WebSocket handler when a new client connects.
	register chan *Client

	// unregister is sent by a Client's ReadPump when its connection closes.
	unregister chan *Client

	// shutdown is closed by Hub.Shutdown() to trigger a forced drain.
	shutdown chan struct{}

	// documentService is used by Run() to persist Yjs updates.
	// Run() is the only caller — no additional locking needed.
	documentService *services.DocumentService

	// done is closed by Run() when the room exits (empty or shutdown).
	// The Hub watches for this to know when to remove the room.
	done chan struct{}
}

// NewRoom constructs a Room for the given document.
// Does not start the Run() goroutine — the Hub does that.
func NewRoom(documentID string, documentService *services.DocumentService) *Room {
	return &Room{
		documentID:      documentID,
		clients:         make(map[*Client]bool),
		broadcast:       make(chan *RoomMessage, 256),
		awareness:       make(chan *RoomMessage, 256),
		register:        make(chan *Client),
		unregister:      make(chan *Client),
		shutdown:        make(chan struct{}),
		documentService: documentService,
		done:            make(chan struct{}),
	}
}

// Close signals this room to send close frames to all clients and exit.
// Called by Hub.Shutdown — never call directly from outside the hub.
func (r *Room) Close() {
	close(r.shutdown)
}

// Run is the event loop for this room.
// It must be started in a goroutine: go room.Run()
//
// It exits when the last client disconnects, or when Close() is called
// during server shutdown. closing r.done signals the Hub.
func (r *Room) Run() {
	defer close(r.done)

	for {
		select {

		// ── new client connected ───────────────────────────────────────────
		case client := <-r.register:
			r.clients[client] = true
			log.Printf("ws: user %s joined doc %s (%d connected)",
				client.userID, r.documentID, len(r.clients))

			// Send SYNC_STEP_1 to new client.
			initMsg := []byte{MsgSync, 0}
			select {
			case client.send <- initMsg:
			default:
				close(client.send)
				delete(r.clients, client)
				break
			}

			// Ask every existing client to re-broadcast their awareness now
			// so the new client sees cursors without waiting for the next edit.
			awarenessQuery := []byte{MsgAwareness, 0x00}
			for existing := range r.clients {
				if existing == client {
					continue
				}
				select {
				case existing.send <- awarenessQuery:
				default:
				}
			}

		// ── client disconnected ────────────────────────────────────────────
		case client := <-r.unregister:
			if _, ok := r.clients[client]; !ok {
				continue
			}

			delete(r.clients, client)
			close(client.send) // causes WritePump to send close frame and exit

			log.Printf("ws: user %s left doc %s (%d remaining)",
				client.userID, r.documentID, len(r.clients))

			// Broadcast a null awareness message so other clients remove
			// this user's cursor from the UI immediately.
			nullAwareness := []byte{MsgAwareness}
			r.fanout(nil, nullAwareness, false)

			if len(r.clients) == 0 {
				log.Printf("ws: room for doc %s is empty, shutting down", r.documentID)
				return
			}

		// ── Yjs document update (persist first, then broadcast) ────────────
		case msg := <-r.broadcast:
			_, err := r.documentService.PersistUpdate(
				context.Background(),
				r.documentID,
				msg.data,
			)
			if err != nil {
				// Persistence failed — do NOT broadcast. Log and continue.
				log.Printf("ws: failed to persist update for doc %s: %v", r.documentID, err)
				continue
			}

			r.fanout(msg.sender, msg.data, true)

		// ── awareness update (cursor / presence — broadcast only) ──────────
		case msg := <-r.awareness:
			r.fanout(msg.sender, msg.data, false)

		// ── graceful shutdown ──────────────────────────────────────────────
		case <-r.shutdown:
			// Send close frames to all connected clients by closing their
			// send channels. WritePump detects the closed channel and sends
			// a WebSocket close frame before its goroutine exits.
			log.Printf("ws: doc room %s received shutdown signal, closing %d client(s)",
				r.documentID, len(r.clients))
			for client := range r.clients {
				close(client.send)
			}
			// Clear the map so the deferred close(r.done) fires immediately
			// without waiting for unregister messages that will never come.
			r.clients = make(map[*Client]bool)
			return
		}
	}
}

// fanout delivers a message to all clients except the sender.
//
// dropOnFull controls behaviour when a client's send buffer is full:
//   - true  (used for document updates): unregister the slow client immediately.
//   - false (used for awareness):        silently drop — cursor lag is acceptable.
//
// sender may be nil (e.g. null awareness on disconnect) — in that case
// the message goes to ALL clients.
func (r *Room) fanout(sender *Client, data []byte, dropOnFull bool) {
	for client := range r.clients {
		if client == sender {
			continue
		}

		select {
		case client.send <- data:
		default:
			if dropOnFull {
				log.Printf("ws: dropping slow client user %s doc %s", client.userID, r.documentID)
				delete(r.clients, client)
				close(client.send)
			}
		}
	}
}

// Size returns the number of clients currently in this room.
func (r *Room) Size() int {
	return len(r.clients)
}
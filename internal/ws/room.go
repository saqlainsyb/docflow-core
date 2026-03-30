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
//   - Destroyed (goroutine exits) when the last client disconnects.
//   - The Hub is notified via Hub.removeRoom so it can clean up its map.
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

	// documentService is used by Run() to persist Yjs updates.
	// Run() is the only caller — no additional locking needed.
	documentService *services.DocumentService

	// done is closed by Run() when the room empties.
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
		documentService: documentService,
		done:            make(chan struct{}),
	}
}

// Run is the event loop for this room.
// It must be started in a goroutine: go room.Run()
//
// It exits when the last client disconnects, closing room.done
// to signal the Hub that this room is ready to be garbage collected.
func (r *Room) Run() {
	defer close(r.done)

	for {
		select {

		// ── new client connected ───────────────────────────────────────────
		case client := <-r.register:
			r.clients[client] = true
			log.Printf("ws: user %s joined doc %s (%d connected)",
				client.userID, r.documentID, len(r.clients))

			// Send SYNC_STEP_1 to new client (unchanged).
			initMsg := []byte{MsgSync, 0}
			select {
			case client.send <- initMsg:
			default:
				close(client.send)
				delete(r.clients, client)
				break
			}

			// NEW: ask every existing client to re-broadcast their awareness now.
			// Without this, the new client waits up to several seconds (or forever
			// if peers are idle) to see other cursors.
			// MsgAwareness + subtype 0x00 is the y-websocket "query awareness" message —
			// recipients respond by immediately re-sending their full awareness state.
			awarenessQuery := []byte{MsgAwareness, 0x00}
			for existing := range r.clients {
				if existing == client {
					continue
				}
				select {
				case existing.send <- awarenessQuery:
				default:
					// Slow/dead — let it be cleaned up naturally on next write failure.
				}
			}

		// ── client disconnected ────────────────────────────────────────────
		case client := <-r.unregister:
			if _, ok := r.clients[client]; !ok {
				// Already removed — nothing to do.
				continue
			}

			delete(r.clients, client)
			close(client.send) // causes WritePump to send close frame and exit

			log.Printf("ws: user %s left doc %s (%d remaining)",
				client.userID, r.documentID, len(r.clients))

			// Broadcast a null awareness message for this client so other
			// clients remove their cursor from the UI immediately.
			nullAwareness := []byte{MsgAwareness}
			r.fanout(nil, nullAwareness, false)

			if len(r.clients) == 0 {
				// Room is empty — exit the goroutine.
				// closing r.done signals the Hub to remove this room from its map.
				log.Printf("ws: room for doc %s is empty, shutting down", r.documentID)
				return
			}

		// ── Yjs document update (must persist first, then broadcast) ───────
		case msg := <-r.broadcast:
			_, err := r.documentService.PersistUpdate(
				context.Background(),
				r.documentID,
				msg.data,
			)
			if err != nil {
				// Persistence failed — do NOT broadcast. Log and continue.
				// The sending client will see no echo; they can retry.
				log.Printf("ws: failed to persist update for doc %s: %v", r.documentID, err)
				continue
			}

			// Persistence succeeded — forward to all other clients.
			r.fanout(msg.sender, msg.data, true)

			// NOTE: snapshot compaction is intentionally disabled in V1.
			// The compactSnapshot call was removed because it passed empty
			// bytes as the merged state, which would silently corrupt
			// documents after every 100 edits by replacing the snapshot
			// with nothing. Updates accumulate in document_updates until
			// V2 implements proper Yjs binary state vector merge.

		// ── awareness update (cursor / presence — broadcast only) ─────────
		case msg := <-r.awareness:
			r.fanout(msg.sender, msg.data, false)
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
			continue // never echo back to the originator
		}

		select {
		case client.send <- data:
			// delivered
		default:
			// Channel full — client is too slow or dead.
			if dropOnFull {
				log.Printf("ws: dropping slow client user %s doc %s", client.userID, r.documentID)
				delete(r.clients, client)
				close(client.send)
			}
			// If !dropOnFull: silently skip — awareness drops are fine.
		}
	}
}

// Size returns the number of clients currently in this room.
// Called by the Hub to report connected counts for cursor colour assignment.
// Note: this is a snapshot read outside of Run() — safe in practice because
// it's only used for a best-effort colour assignment, not for correctness.
func (r *Room) Size() int {
	return len(r.clients)
}

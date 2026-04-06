// internal/ws/hub.go
package ws

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/saqlainsyb/docflow-core/internal/services"
)

// Hub is the server-wide singleton that manages all active Rooms.
//
// Two separate room maps:
//   - docRooms:   one Room per open document  (/ws/documents/:id)
//   - boardRooms: one Room per open board     (/ws/boards/:id)
//
// The Hub itself does not run a goroutine — room lifecycle is managed
// inline using a mutex. Each Room runs its own Run() goroutine.
type Hub struct {
	// docRooms maps documentID -> *Room for document WebSocket connections.
	docRooms map[string]*Room
	docMu    sync.RWMutex

	// boardRooms maps boardID -> *BoardRoom for board event connections.
	boardRooms map[string]*BoardRoom
	boardMu    sync.RWMutex

	// documentService is injected so Rooms can call PersistUpdate.
	documentService *services.DocumentService
}

// NewHub constructs the Hub. Call once at startup in main.go.
func NewHub(documentService *services.DocumentService) *Hub {
	return &Hub{
		docRooms:        make(map[string]*Room),
		boardRooms:      make(map[string]*BoardRoom),
		documentService: documentService,
	}
}

// ── Document rooms ────────────────────────────────────────────────────────────

// GetOrCreateDocRoom returns the existing Room for a document, or creates
// a new one and starts its Run() goroutine if none exists yet.
// Called by the WebSocket handler when a client connects to a document.
func (h *Hub) GetOrCreateDocRoom(documentID string) *Room {
	// Fast path: room already exists — read lock only.
	h.docMu.RLock()
	room, ok := h.docRooms[documentID]
	h.docMu.RUnlock()
	if ok {
		return room
	}

	// Slow path: need to create — take write lock.
	h.docMu.Lock()
	defer h.docMu.Unlock()

	// Double-check: another goroutine may have created it between our two locks.
	if room, ok = h.docRooms[documentID]; ok {
		return room
	}

	room = NewRoom(documentID, h.documentService)
	h.docRooms[documentID] = room

	// Start the room's event loop. It will close room.done when it exits.
	go room.Run()

	// Watch for the room to empty so we can remove it from the map.
	go h.watchDocRoom(documentID, room)

	log.Printf("hub: created doc room for document %s", documentID)
	return room
}

// watchDocRoom blocks until the room's Run() goroutine exits (room.done closes),
// then removes it from the Hub's map so it can be garbage collected.
func (h *Hub) watchDocRoom(documentID string, room *Room) {
	<-room.done // blocks until Room.Run() returns

	h.docMu.Lock()
	defer h.docMu.Unlock()

	// Only delete if it's still the same room (not a newer replacement).
	if h.docRooms[documentID] == room {
		delete(h.docRooms, documentID)
		log.Printf("hub: removed doc room for document %s", documentID)
	}
}

// RoomSize returns the number of clients currently connected to a document room.
// Returns 0 if no room exists (no one connected yet).
// Called by DocumentService.IssueToken to assign cursor colours round-robin.
func (h *Hub) RoomSize(documentID string) int {
	h.docMu.RLock()
	defer h.docMu.RUnlock()

	if room, ok := h.docRooms[documentID]; ok {
		return room.Size()
	}
	return 0
}

// ── Board rooms ───────────────────────────────────────────────────────────────

// GetOrCreateBoardRoom returns the existing BoardRoom for a board, or creates one.
// Called by the WebSocket handler when a client connects to a board.
func (h *Hub) GetOrCreateBoardRoom(boardID string) *BoardRoom {
	h.boardMu.RLock()
	room, ok := h.boardRooms[boardID]
	h.boardMu.RUnlock()
	if ok {
		return room
	}

	h.boardMu.Lock()
	defer h.boardMu.Unlock()

	if room, ok = h.boardRooms[boardID]; ok {
		return room
	}

	room = NewBoardRoom(boardID)
	h.boardRooms[boardID] = room

	go room.Run()
	go h.watchBoardRoom(boardID, room)

	log.Printf("hub: created board room for board %s", boardID)
	return room
}

// watchBoardRoom blocks until the BoardRoom empties, then cleans up the map.
func (h *Hub) watchBoardRoom(boardID string, room *BoardRoom) {
	<-room.done

	h.boardMu.Lock()
	defer h.boardMu.Unlock()

	if h.boardRooms[boardID] == room {
		delete(h.boardRooms, boardID)
		log.Printf("hub: removed board room for board %s", boardID)
	}
}

// BroadcastToBoard serialises a board event payload to JSON and delivers it
// to all clients currently in the board room.
func (h *Hub) BroadcastToBoard(boardID string, payload any) {
	h.boardMu.RLock()
	room, ok := h.boardRooms[boardID]
	h.boardMu.RUnlock()

	if !ok {
		return
	}

	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("hub: failed to marshal board event for board %s: %v", boardID, err)
		return
	}

	room.broadcast <- data
}

// ── Graceful shutdown ─────────────────────────────────────────────────────────

// Shutdown closes every active document and board room, sends WebSocket close
// frames to all connected clients, and waits for all room goroutines to exit.
//
// It respects the provided context deadline — if rooms do not drain within
// the deadline, Shutdown returns anyway so the process can exit cleanly.
//
// Call order in main.go:
//
//	server.Shutdown(httpCtx)  // stop accepting new connections first
//	hub.Shutdown(wsCtx)       // then drain WebSocket rooms
//	redisClient.Close()       // then close backing resources
//	dbPool.Close()
func (h *Hub) Shutdown(ctx context.Context) {
	// Snapshot both room maps under their respective locks, then release
	// immediately. We don't want to hold locks while sending close frames
	// or waiting on done channels — that would deadlock watchDocRoom /
	// watchBoardRoom which also need to acquire the write lock.
	h.docMu.RLock()
	docRooms := make([]*Room, 0, len(h.docRooms))
	for _, r := range h.docRooms {
		docRooms = append(docRooms, r)
	}
	h.docMu.RUnlock()

	h.boardMu.RLock()
	boardRooms := make([]*BoardRoom, 0, len(h.boardRooms))
	for _, r := range h.boardRooms {
		boardRooms = append(boardRooms, r)
	}
	h.boardMu.RUnlock()

	total := len(docRooms) + len(boardRooms)
	log.Printf("hub: shutting down %d doc room(s) and %d board room(s)",
		len(docRooms), len(boardRooms))

	if total == 0 {
		return
	}

	// Signal each room to close. Each room's Close() method sends a WebSocket
	// close frame to every connected client, which causes their ReadPumps to
	// return errors and send to the unregister channel, which drains the room,
	// which causes Run() to return, which closes room.done.
	for _, r := range docRooms {
		r.Close()
	}
	for _, r := range boardRooms {
		r.Close()
	}

	// Wait for all room goroutines to finish, bounded by the context deadline.
	// We use a WaitGroup fan-out so all rooms drain concurrently rather than
	// sequentially — the total wait is bounded by the slowest room, not the sum.
	var wg sync.WaitGroup

	for _, r := range docRooms {
		wg.Add(1)
		go func(room *Room) {
			defer wg.Done()
			select {
			case <-room.done:
			case <-ctx.Done():
				log.Printf("hub: timed out waiting for doc room %s to drain", room.documentID)
			}
		}(r)
	}

	for _, r := range boardRooms {
		wg.Add(1)
		go func(room *BoardRoom) {
			defer wg.Done()
			select {
			case <-room.done:
			case <-ctx.Done():
				log.Printf("hub: timed out waiting for board room %s to drain", room.boardID)
			}
		}(r)
	}

	wg.Wait()
}

// ── BoardRoom ─────────────────────────────────────────────────────────────────

// BoardRoom manages clients connected to the board event stream (/ws/boards/:id).
// Unlike document Rooms, board rooms only broadcast JSON — no persistence.
type BoardRoom struct {
	boardID    string
	clients    map[*BoardClient]bool
	broadcast  chan []byte
	register   chan *BoardClient
	unregister chan *BoardClient
	// shutdown signals Run() to close all clients and exit.
	shutdown chan struct{}
	done     chan struct{}
}

// BoardClient represents one browser tab connected to the board event stream.
type BoardClient struct {
	conn   *websocket.Conn
	send   chan []byte
	userID string
	name   string
}

// NewBoardClient constructs a BoardClient after a successful WebSocket upgrade.
func NewBoardClient(conn *websocket.Conn, userID string, name string) *BoardClient {
	return &BoardClient{
		conn:   conn,
		send:   make(chan []byte, 256),
		userID: userID,
		name:   name,
	}
}

// ReadPump pumps messages from the board WebSocket connection.
// Board clients only receive — they don't send Yjs updates.
// We still need the read loop running to detect disconnects and handle pings.
func (c *BoardClient) ReadPump(room *BoardRoom) {
	defer func() {
		room.unregister <- c
	}()

	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
	}
}

// WritePump pumps outbound board events from c.send to the WebSocket connection.
func (c *BoardClient) WritePump() {
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
				// send channel closed — send WebSocket close frame and exit.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
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

// NewBoardRoom constructs a BoardRoom.
func NewBoardRoom(boardID string) *BoardRoom {
	return &BoardRoom{
		boardID:    boardID,
		clients:    make(map[*BoardClient]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *BoardClient),
		unregister: make(chan *BoardClient),
		shutdown:   make(chan struct{}),
		done:       make(chan struct{}),
	}
}

// Close signals this room to send close frames to all clients and exit.
// Called by Hub.Shutdown — never call directly from outside the hub.
func (r *BoardRoom) Close() {
	close(r.shutdown)
}

// Run is the event loop for the board room. Same single-writer pattern as Room.
func (r *BoardRoom) Run() {
	defer close(r.done)

	for {
		select {
		case client := <-r.register:
			r.clients[client] = true
			joinMsg, _ := json.Marshal(map[string]any{
				"type":    EvtUserJoined,
				"user_id": client.userID,
				"name":    client.name,
			})
			for c := range r.clients {
				if c != client {
					select {
					case c.send <- joinMsg:
					default:
					}
				}
			}

		case client := <-r.unregister:
			if _, ok := r.clients[client]; !ok {
				continue
			}
			delete(r.clients, client)
			close(client.send)
			log.Printf("hub: user %s left board room %s (%d remaining)",
				client.userID, r.boardID, len(r.clients))

			if len(r.clients) == 0 {
				log.Printf("hub: board room %s is empty, shutting down", r.boardID)
				return
			}

		case data := <-r.broadcast:
			for client := range r.clients {
				select {
				case client.send <- data:
				default:
					// Slow client — drop and disconnect.
					delete(r.clients, client)
					close(client.send)
				}
			}

		case <-r.shutdown:
			// Graceful shutdown: close every client's send channel.
			// WritePump sees the closed channel and sends a WebSocket close frame,
			// which causes the peer to close its end, which causes ReadPump to
			// return an error and send to unregister — but we're already shutting
			// down so we don't need to process those unregisters.
			log.Printf("hub: board room %s received shutdown signal, closing %d client(s)",
				r.boardID, len(r.clients))
			for client := range r.clients {
				close(client.send)
			}
			// Clear the map so the deferred close(r.done) fires immediately.
			r.clients = make(map[*BoardClient]bool)
			return
		}
	}
}

// Register adds a client to the room. Called by the WebSocket handler.
func (r *Room) Register(client *Client) {
	r.register <- client
}

// Register adds a board client to the board room.
func (r *BoardRoom) Register(client *BoardClient) {
	r.register <- client
}
// internal/ws/hub.go
package ws

import (
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
//
// Called by card and column services when mutations happen
// (replaces the // TODO stubs in services/cards.go and services/columns.go).
//
// payload must be a struct with a "type" field — e.g.:
//
//	hub.BroadcastToBoard(boardID, map[string]any{
//	    "type": ws.EvtCardMoved,
//	    "card_id": cardID,
//	    "column_id": columnID,
//	    "position": position,
//	})
//
// If no room exists for the board (nobody is connected), this is a no-op.
func (h *Hub) BroadcastToBoard(boardID string, payload any) {
	h.boardMu.RLock()
	room, ok := h.boardRooms[boardID]
	h.boardMu.RUnlock()

	if !ok {
		// No one is connected to this board — nothing to broadcast.
		return
	}

	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("hub: failed to marshal board event for board %s: %v", boardID, err)
		return
	}

	room.broadcast <- data
}

// ── BoardRoom ─────────────────────────────────────────────────────────────────

// BoardRoom manages clients connected to the board event stream (/ws/boards/:id).
// Unlike document Rooms, board rooms only broadcast JSON — no persistence.
//
// Each client here is a plain browser tab with the board view open.
// When a card moves, the hub calls BroadcastToBoard and every connected
// tab updates its UI without polling.
type BoardRoom struct {
	boardID    string
	clients    map[*BoardClient]bool
	broadcast  chan []byte
	register   chan *BoardClient
	unregister chan *BoardClient
	done       chan struct{}
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
		// We don't process incoming messages from board clients —
		// boards are server-push only. But we must keep reading
		// to detect close frames and trigger unregister.
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
		done:       make(chan struct{}),
	}
}

// Run is the event loop for the board room. Same single-writer pattern as Room.
func (r *BoardRoom) Run() {
	defer close(r.done)

	for {
		select {
		case client := <-r.register:
			r.clients[client] = true
			// Broadcast join to everyone except the joiner
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

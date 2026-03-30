// internal/ws/protocol.go
package ws

// Document WebSocket protocol — first byte of every binary message.
// These constants match the y-websocket protocol that the frontend
// TipTap / Yjs client sends and expects.
const (
	// MsgSync is used for the Yjs sync handshake and ongoing document updates.
	// Flow:
	//   new client connects → server sends SYNC_STEP_1 (state vector)
	//   client replies with SYNC_STEP_2 (updates server hasn't seen)
	//   server replies with all updates client hasn't seen
	//   both sides now have identical document state
	MsgSync = byte(0x00)

	// MsgAwareness carries ephemeral presence state: cursor position, user name,
	// assigned colour. Broadcast-only — never persisted to the database.
	MsgAwareness = byte(0x01)

	// MsgClose is sent by the server before it intentionally closes a connection,
	// e.g. during graceful shutdown or when a token expires.
	MsgClose = byte(0x02)

	MsgAwarenessQuery = 0x03
)

// Board WebSocket event type strings.
// Every message on the board room (/ws/boards/:id) is a JSON object
// with a "type" field matching one of these constants.
// The frontend switches on this field to update its local state.
const (
	EvtCardCreated     = "CARD_CREATED"
	EvtCardUpdated     = "CARD_UPDATED"
	EvtCardMoved       = "CARD_MOVED"
	EvtCardArchived    = "CARD_ARCHIVED"
	EvtCardUnarchived = "CARD_UNARCHIVED"
	EvtCardDeleted     = "CARD_DELETED"
	EvtColumnCreated   = "COLUMN_CREATED"
	EvtColumnRenamed   = "COLUMN_RENAMED"
	EvtColumnReordered = "COLUMN_REORDERED"
	EvtColumnDeleted   = "COLUMN_DELETED"
)
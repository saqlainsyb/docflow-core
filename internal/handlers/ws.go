// internal/handlers/ws.go
package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/saqlainsyb/docflow-core/internal/config"
	"github.com/saqlainsyb/docflow-core/internal/utils"
	ws "github.com/saqlainsyb/docflow-core/internal/ws"
)

// upgrader converts an incoming HTTP request into a WebSocket connection.
// CheckOrigin is permissive here — production origin checking is handled
// by the CORS middleware on the HTTP layer before upgrade.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// WSHandler handles both WebSocket endpoints.
// It holds the hub (room manager) and config (for JWT secrets).
type WSHandler struct {
	hub *ws.Hub
	cfg *config.Config
}

// NewWSHandler constructs the WebSocket handler.
func NewWSHandler(hub *ws.Hub, cfg *config.Config) *WSHandler {
	return &WSHandler{hub: hub, cfg: cfg}
}

// HandleDocumentWS handles: WS /ws/documents/:id?token=<document_jwt>
//
// Auth flow:
//  1. Read the document JWT from the ?token= query param.
//  2. Validate it using JWT_DOCUMENT_SECRET — this is a different secret
//     from access tokens, scoped only to document WebSocket connections.
//  3. Verify the document ID in the token matches the :id in the URL.
//     This prevents a token issued for document A being used to join document B.
//  4. Upgrade the HTTP connection to WebSocket.
//  5. Get or create the document room from the hub.
//  6. Create a Client, register it, start its goroutines.
func (h *WSHandler) HandleDocumentWS(c *gin.Context) {
	documentID := c.Param("id")
	tokenString := c.Query("token")

	if tokenString == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "MISSING_TOKEN", "document token is required")
		return
	}

	// Validate using the document-specific secret (not the access token secret).
	claims, err := utils.ValidateToken(tokenString, h.cfg.JWTDocumentSecret)
	if err != nil {
		utils.ErrorResponse(c, http.StatusUnauthorized, "INVALID_TOKEN", "invalid or expired document token")
		return
	}

	// Reject tokens not issued for WebSocket document access.
	if claims.TokenType != "document" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "INVALID_TOKEN", "wrong token type")
		return
	}

	// Reject tokens whose document ID doesn't match the URL.
	// Without this check, one valid token could open any document room.
	if claims.DocumentID != documentID {
		utils.ErrorResponse(c, http.StatusForbidden, "BOARD_ACCESS_DENIED", "token not valid for this document")
		return
	}

	// All checks passed — upgrade the HTTP connection to WebSocket.
	// After this call, HTTP is done. Any further errors must be sent
	// over the WebSocket connection, not as HTTP responses.
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// Upgrade failed before the connection switched protocols —
		// gorilla already wrote an HTTP error response.
		log.Printf("ws: document upgrade failed for user %s: %v", claims.UserID, err)
		return
	}

	// Get or create the room for this document.
	room := h.hub.GetOrCreateDocRoom(documentID)

	// Create the client with identity from the validated token.
	client := ws.NewClient(conn, room, claims.UserID, claims.Color, documentID)

	// Register the client with the room's event loop.
	// Room.Run() will send the Yjs SYNC_STEP_1 initiation immediately after.
	room.Register(client)

	// Start the write pump in a background goroutine.
	// It drains client.send and writes to the connection.
	go client.WritePump()

	// ReadPump runs on this goroutine (the Gin handler goroutine).
	// It blocks until the connection closes, reading messages and
	// routing them to the room. When it returns, the handler is done.
	client.ReadPump()
}

// HandleBoardWS handles: WS /ws/boards/:id?token=<access_jwt>
//
// Auth flow:
//  1. Read the standard access JWT from the ?token= query param.
//     (Browser WebSocket API cannot set headers, so token goes in URL.)
//  2. Validate it using JWT_ACCESS_SECRET.
//  3. Upgrade the connection.
//  4. Get or create the board room.
//  5. Create a BoardClient, register it, start its goroutines.
//
// Board rooms are server-push only — clients receive JSON events when
// cards or columns are mutated. They don't send anything meaningful back.
func (h *WSHandler) HandleBoardWS(c *gin.Context) {
	boardID := c.Param("id")
	tokenString := c.Query("token")

	if tokenString == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "MISSING_TOKEN", "access token is required")
		return
	}

	// Standard access token — uses the access secret, not the document secret.
	claims, err := utils.ValidateToken(tokenString, h.cfg.JWTAccessSecret)
	if err != nil {
		utils.ErrorResponse(c, http.StatusUnauthorized, "INVALID_TOKEN", "invalid or expired access token")
		return
	}

	if claims.TokenType != "access" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "INVALID_TOKEN", "wrong token type")
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws: board upgrade failed for user %s board %s: %v", claims.UserID, boardID, err)
		return
	}

	room := h.hub.GetOrCreateBoardRoom(boardID)

	client := ws.NewBoardClient(conn, claims.UserID)

	room.Register(client)

	go client.WritePump()

	client.ReadPump(room)
}
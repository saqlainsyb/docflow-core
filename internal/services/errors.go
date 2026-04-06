// internal/services/errors.go
package services

import "errors"

// Auth errors
var (
	ErrEmailAlreadyExists  = errors.New("email already exists")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrRefreshTokenExpired = errors.New("refresh token expired")
	ErrTokenTheftDetected  = errors.New("token theft detected")
	ErrWeakPassword        = errors.New("password does not meet requirements")
)

// Resource errors
var (
	ErrNotFound     = errors.New("resource not found")
	ErrForbidden    = errors.New("forbidden")
	ErrConflict     = errors.New("conflict")
	ErrUserNotFound = errors.New("user not found")
)

// Workspace errors
var (
	ErrAlreadyMember           = errors.New("user is already a workspace member")
	ErrCannotRemoveOwner       = errors.New("cannot remove the workspace owner")
	ErrCannotChangeSelfRole    = errors.New("cannot change your own role")
	ErrInsufficientPermissions = errors.New("insufficient permissions")
)

// Board errors
var (
	ErrBoardAccessDenied      = errors.New("board access denied")
	ErrAlreadyBoardMember     = errors.New("user is already a board member")
	ErrCannotRemoveBoardOwner = errors.New("cannot remove the board owner — transfer ownership first")
	ErrTargetNotBoardMember   = errors.New("target user must be an existing board member")
)

// Invitation errors
var (
	// ErrInvitationInvalid covers token-not-found, already-accepted, and cancelled.
	// We intentionally collapse these into one error code to avoid leaking
	// information about which tokens exist in the system.
	ErrInvitationInvalid = errors.New("invitation is invalid or has already been used")

	// ErrInvitationExpired is separated from Invalid so the frontend can
	// show a more helpful "this link has expired" message.
	ErrInvitationExpired = errors.New("invitation has expired")

	// ErrInvitationAlreadyPending prevents spamming invite emails to the same
	// address. The existing pending invite must be cancelled first.
	ErrInvitationAlreadyPending = errors.New("a pending invitation for this email already exists")

	// ErrInvitationEmailMismatch is returned when the accepting user's email
	// does not match the invitation target email.
	ErrInvitationEmailMismatch = errors.New("this invitation was sent to a different email address")

	// ErrEmailDeliveryFailed wraps Resend errors — the invitation row is created
	// but the email could not be delivered.
	ErrEmailDeliveryFailed = errors.New("invitation created but email delivery failed")
)
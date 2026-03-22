package services

import "errors"

// Auth errors
var (
	ErrEmailAlreadyExists  = errors.New("email already exists")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrRefreshTokenExpired = errors.New("refresh token expired")
	ErrTokenTheftDetected  = errors.New("token theft detected")
	ErrWeakPassword = errors.New("password does not meet requirements")
)

// Resource errors
var (
	ErrNotFound            = errors.New("resource not found")
	ErrForbidden           = errors.New("forbidden")
	ErrConflict            = errors.New("conflict")
	ErrUserNotFound        = errors.New("user not found")
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
	ErrBoardAccessDenied = errors.New("board access denied")
)
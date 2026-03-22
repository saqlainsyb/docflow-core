package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/saqlainsyb/docflow-core/internal/models"
	"github.com/saqlainsyb/docflow-core/internal/services"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

type AuthHandler struct {
	authService *services.AuthService
}

func NewAuthHandler(authService *services.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// Register handles POST /api/v1/auth/register
// Validates input, calls the auth service, returns the user and token pair.
func (h *AuthHandler) Register(c *gin.Context) {
	var req models.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	resp, err := h.authService.Register(c.Request.Context(), req)
	if err != nil {
		handleAuthError(c, err)
		return
	}

	c.JSON(http.StatusCreated, resp)
}

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	resp, err := h.authService.Login(c.Request.Context(), req)
	if err != nil {
		handleAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// Refresh handles POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req models.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	resp, err := h.authService.Refresh(c.Request.Context(), req)
	if err != nil {
		handleAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// Logout handles POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	var req models.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if err := h.authService.Logout(c.Request.Context(), req); err != nil {
		utils.ErrInternal(c)
		return
	}

	c.Status(http.StatusNoContent)
}

// GetMe handles GET /api/v1/users/me
// Returns the currently authenticated user's profile.
// The user_id is injected into context by the auth middleware.
func (h *AuthHandler) GetMe(c *gin.Context) {
	userID := c.GetString("user_id")

	user, err := h.authService.GetMe(c.Request.Context(), userID)
	if err != nil {
		utils.ErrInternal(c)
		return
	}

	c.JSON(http.StatusOK, user)
}

// UpdateMe handles PATCH /api/v1/users/me
// Updates the authenticated user's name and/or avatar_url.
// Both fields are optional — send only what you want to change.
func (h *AuthHandler) UpdateMe(c *gin.Context) {
	var req models.UpdateMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	userID := c.GetString("user_id")

	user, err := h.authService.UpdateMe(c.Request.Context(), userID, req)
	if err != nil {
		handleAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, user)
}

// handleAuthError maps service errors to the correct HTTP response.
// This keeps error mapping in one place instead of scattered across handlers.
func handleAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrEmailAlreadyExists):
		utils.ErrConflict(c, "EMAIL_ALREADY_EXISTS", "an account with this email already exists")
	case errors.Is(err, services.ErrInvalidCredentials):
		utils.ErrUnauthorized(c, "INVALID_CREDENTIALS", "invalid email or password")
	case errors.Is(err, services.ErrInvalidRefreshToken):
		utils.ErrUnauthorized(c, "REFRESH_TOKEN_INVALID", "refresh token is invalid or has been revoked")
	case errors.Is(err, services.ErrRefreshTokenExpired):
		utils.ErrUnauthorized(c, "REFRESH_TOKEN_EXPIRED", "refresh token has expired")
	case errors.Is(err, services.ErrTokenTheftDetected):
		utils.ErrUnauthorized(c, "TOKEN_THEFT_DETECTED", "suspicious activity detected, please login again")
	case errors.Is(err, services.ErrUserNotFound):
		utils.ErrorResponse(c, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
	case errors.Is(err, services.ErrWeakPassword):
		utils.ErrorResponse(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		utils.ErrInternal(c)
	}
}

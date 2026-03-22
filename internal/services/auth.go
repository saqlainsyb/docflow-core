package services

import (
	"context"
	"errors"
	"time"
	"fmt"

	"github.com/saqlainsyb/docflow-core/internal/config"
	"github.com/saqlainsyb/docflow-core/internal/models"
	"github.com/saqlainsyb/docflow-core/internal/repositories"
	"github.com/saqlainsyb/docflow-core/internal/utils"
)

type AuthService struct {
	userRepo         *repositories.UserRepository
	refreshTokenRepo *repositories.RefreshTokenRepository
	workspaceRepo    *repositories.WorkspaceRepository
	cfg              *config.Config
}

func NewAuthService(
	userRepo *repositories.UserRepository,
	refreshTokenRepo *repositories.RefreshTokenRepository,
	workspaceRepo *repositories.WorkspaceRepository,
	cfg *config.Config,
) *AuthService {
	return &AuthService{
		userRepo:         userRepo,
		refreshTokenRepo: refreshTokenRepo,
		workspaceRepo:    workspaceRepo,
		cfg:              cfg,
	}
}

// Register creates a new user account.
// Steps:
// 1. Check email not already taken
// 2. Hash the password
// 3. Create the user
// 4. Issue token pair
func (s *AuthService) Register(ctx context.Context, req models.RegisterRequest) (*models.AuthResponse, error) {

	// check if email already exists
	_, err := s.userRepo.FindByEmail(ctx, req.Email)
	if err == nil {
		// found a user — email is taken
		return nil, ErrEmailAlreadyExists
	}
	if !errors.Is(err, repositories.ErrNotFound) {
		// something else went wrong
		return nil, err
	}

	// validate password strength — binding tag only checks min length
if ok, reason := utils.ValidatePassword(req.Password); !ok {
    return nil, fmt.Errorf("%w: %s", ErrWeakPassword, reason)
}

	// hash the password — never store plain text
	hash, err := utils.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	// create the user
	user, err := s.userRepo.Create(ctx, req.Email, hash, req.Name)
	if err != nil {
		return nil, err
	}

	// auto-create personal workspace — every user gets one on registration
	// name format: "{name}'s Workspace"
	_, err = s.workspaceRepo.CreateWithOwner(ctx, req.Name+"'s Workspace", user.ID)
	if err != nil {
		return nil, err
	}

	// issue token pair
	return s.issueTokenPair(ctx, user)
}

// Login authenticates a user with email and password.
// Steps:
// 1. Find user by email
// 2. Check password
// 3. Revoke all existing refresh tokens
// 4. Issue new token pair
func (s *AuthService) Login(ctx context.Context, req models.LoginRequest) (*models.AuthResponse, error) {

	// find user by email
	user, err := s.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		// whether the email doesn't exist or something else went wrong,
		// we always return the same error — never reveal which
		return nil, ErrInvalidCredentials
	}

	// check password against stored hash
	if err := utils.CheckPassword(req.Password, user.PasswordHash); err != nil {
		return nil, ErrInvalidCredentials
	}

	// revoke all existing sessions before issuing new ones
	if err := s.refreshTokenRepo.RevokeAllForUser(ctx, user.ID); err != nil {
		return nil, err
	}

	// issue new token pair
	return s.issueTokenPair(ctx, user)
}

// Refresh validates a refresh token and issues a new token pair.
// Steps:
// 1. Hash the incoming token and look it up
// 2. Check it is not expired or revoked
// 3. Theft detection — if already revoked, kill all sessions
// 4. Revoke the used token
// 5. Issue new token pair
func (s *AuthService) Refresh(ctx context.Context, req models.RefreshRequest) (*models.AuthResponse, error) {

	// hash the raw token to look it up in the database
	tokenHash := utils.HashToken(req.RefreshToken)

	storedToken, err := s.refreshTokenRepo.FindByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, err
	}

	// token theft detection — if this token was already revoked and
	// someone is trying to use it again, an attacker may have stolen it
	if storedToken.Revoked {
		// kill ALL sessions for this user immediately
		_ = s.refreshTokenRepo.RevokeAllForUser(ctx, storedToken.UserID)
		return nil, ErrTokenTheftDetected
	}

	// check expiry
	if time.Now().After(storedToken.ExpiresAt) {
		return nil, ErrRefreshTokenExpired
	}

	// revoke the used token — it can never be used again
	if err := s.refreshTokenRepo.Revoke(ctx, storedToken.ID); err != nil {
		return nil, err
	}

	// load the user to get their current details
	user, err := s.userRepo.FindByID(ctx, storedToken.UserID)
	if err != nil {
		return nil, err
	}

	// issue fresh token pair
	return s.issueTokenPair(ctx, user)
}

// Logout revokes a refresh token.
// Returns nil even if the token is not found — we never reveal token validity.
func (s *AuthService) Logout(ctx context.Context, req models.RefreshRequest) error {
	tokenHash := utils.HashToken(req.RefreshToken)

	storedToken, err := s.refreshTokenRepo.FindByHash(ctx, tokenHash)
	if err != nil {
		// token not found — treat as success, don't leak information
		return nil
	}

	return s.refreshTokenRepo.Revoke(ctx, storedToken.ID)
}

// issueTokenPair generates an access token and refresh token for a user.
// This is called at the end of register, login, and refresh.
// It is a private method — only this service uses it.
func (s *AuthService) issueTokenPair(ctx context.Context, user *models.User) (*models.AuthResponse, error) {

	// generate access token — short lived JWT
	accessToken, err := utils.GenerateAccessToken(
		user.ID,
		user.Email,
		user.Name,
		s.cfg.JWTAccessSecret,
		s.cfg.JWTAccessExpiry,
	)
	if err != nil {
		return nil, err
	}

	// generate refresh token — random 32 bytes
	rawRefreshToken, err := utils.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	// hash and store the refresh token
	tokenHash := utils.HashToken(rawRefreshToken)
	expiresAt := time.Now().Add(s.cfg.JWTRefreshExpiry)

	if err := s.refreshTokenRepo.Create(ctx, user.ID, tokenHash, expiresAt); err != nil {
		return nil, err
	}

	return &models.AuthResponse{
		User: models.UserPublic{
			ID:        user.ID,
			Email:     user.Email,
			Name:      user.Name,
			AvatarURL: user.AvatarURL,
			CreatedAt: user.CreatedAt,
		},
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
	}, nil
}

// GetMe returns the public profile of the currently authenticated user.
func (s *AuthService) GetMe(ctx context.Context, userID string) (*models.UserPublic, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &models.UserPublic{
		ID:        user.ID,
		Email:     user.Email,
		Name:      user.Name,
		AvatarURL: user.AvatarURL,
		CreatedAt: user.CreatedAt,
	}, nil
}

// UpdateMe updates the authenticated user's name and/or avatar_url.
// Both fields are optional — only provided fields are changed.
func (s *AuthService) UpdateMe(ctx context.Context, userID string, req models.UpdateMeRequest) (*models.UserPublic, error) {
	user, err := s.userRepo.Update(ctx, userID, req.Name, req.AvatarURL)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return user, nil
}
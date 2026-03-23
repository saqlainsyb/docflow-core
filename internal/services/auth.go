package services

import (
	"context"
	"errors"
	"fmt"
	"time"

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
// 2. Validate password strength
// 3. Hash the password
// 4. Create the user + personal workspace
// 5. Issue token pair
// The raw refresh token is returned inside AuthResponse.RefreshToken (json:"-").
// The handler is responsible for writing it to an HttpOnly cookie.
func (s *AuthService) Register(ctx context.Context, req models.RegisterRequest) (*models.AuthResponse, error) {
	_, err := s.userRepo.FindByEmail(ctx, req.Email)
	if err == nil {
		return nil, ErrEmailAlreadyExists
	}
	if !errors.Is(err, repositories.ErrNotFound) {
		return nil, err
	}

	if ok, reason := utils.ValidatePassword(req.Password); !ok {
		return nil, fmt.Errorf("%w: %s", ErrWeakPassword, reason)
	}

	hash, err := utils.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	user, err := s.userRepo.Create(ctx, req.Email, hash, req.Name)
	if err != nil {
		return nil, err
	}

	_, err = s.workspaceRepo.CreateWithOwner(ctx, req.Name+"'s Workspace", user.ID)
	if err != nil {
		return nil, err
	}

	return s.issueTokenPair(ctx, user)
}

// Login authenticates a user with email and password.
// Steps:
// 1. Find user by email
// 2. Check password
// 3. Revoke all existing refresh tokens (single active session per login)
// 4. Issue new token pair
// The raw refresh token is returned inside AuthResponse.RefreshToken (json:"-").
// The handler is responsible for writing it to an HttpOnly cookie.
func (s *AuthService) Login(ctx context.Context, req models.LoginRequest) (*models.AuthResponse, error) {
	user, err := s.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		// always return the same error — never reveal whether the email exists
		return nil, ErrInvalidCredentials
	}

	if err := utils.CheckPassword(req.Password, user.PasswordHash); err != nil {
		return nil, ErrInvalidCredentials
	}

	if err := s.refreshTokenRepo.RevokeAllForUser(ctx, user.ID); err != nil {
		return nil, err
	}

	return s.issueTokenPair(ctx, user)
}

// Refresh validates a refresh token and issues a new token pair.
// rawToken is the plain token value read from the HttpOnly cookie by the handler.
// Steps:
// 1. Hash and look up the token
// 2. Theft detection — revoked token reused means attacker may have it
// 3. Check expiry
// 4. Revoke the used token immediately (rotation)
// 5. Issue new token pair
// The new raw refresh token is returned inside AuthResponse.RefreshToken (json:"-").
// The handler overwrites the cookie with the new value.
func (s *AuthService) Refresh(ctx context.Context, rawToken string) (*models.AuthResponse, error) {
	tokenHash := utils.HashToken(rawToken)

	storedToken, err := s.refreshTokenRepo.FindByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, err
	}

	// theft detection — already-revoked token presented again
	if storedToken.Revoked {
		_ = s.refreshTokenRepo.RevokeAllForUser(ctx, storedToken.UserID)
		return nil, ErrTokenTheftDetected
	}

	if time.Now().After(storedToken.ExpiresAt) {
		return nil, ErrRefreshTokenExpired
	}

	if err := s.refreshTokenRepo.Revoke(ctx, storedToken.ID); err != nil {
		return nil, err
	}

	user, err := s.userRepo.FindByID(ctx, storedToken.UserID)
	if err != nil {
		return nil, err
	}

	return s.issueTokenPair(ctx, user)
}

// Logout revokes a refresh token.
// rawToken is the plain token value read from the HttpOnly cookie by the handler.
// Returns nil even if the token is not found — we never reveal token validity.
// The handler clears the cookie regardless of what this returns.
func (s *AuthService) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}

	tokenHash := utils.HashToken(rawToken)

	storedToken, err := s.refreshTokenRepo.FindByHash(ctx, tokenHash)
	if err != nil {
		// not found — treat as already logged out, don't leak information
		return nil
	}

	return s.refreshTokenRepo.Revoke(ctx, storedToken.ID)
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

// issueTokenPair generates a new access + refresh token pair for a user.
// Stores the refresh token hash in the database.
// Returns AuthResponse with RefreshToken set (json:"-") — handler writes it to cookie.
func (s *AuthService) issueTokenPair(ctx context.Context, user *models.User) (*models.AuthResponse, error) {
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

	rawRefreshToken, err := utils.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

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
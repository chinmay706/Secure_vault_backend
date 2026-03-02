package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"securevault-backend/src/models"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/api/idtoken"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserExists         = errors.New("user already exists")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrTokenExpired       = errors.New("token has expired")
)

// AuthService handles authentication-related operations
type AuthService struct {
	db        *sql.DB
	jwtSecret []byte
}

// NewAuthService creates a new AuthService
func NewAuthService(db *sql.DB, jwtSecret string) *AuthService {
	return &AuthService{
		db:        db,
		jwtSecret: []byte(jwtSecret),
	}
}

// Claims represents JWT token claims
type Claims struct {
	UserID uuid.UUID        `json:"user_id"`
	Email  string           `json:"email"`
	Role   models.UserRole  `json:"role"`
	jwt.RegisteredClaims
}

// SignUpRequest represents signup request data
type SignUpRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginRequest represents login request data
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse represents authentication response
type AuthResponse struct {
	User  *models.User `json:"user"`
	Token string       `json:"token"`
}

// SignUp creates a new user account
func (s *AuthService) SignUp(req *SignUpRequest) (*AuthResponse, error) {
	if req.Email == "" {
		return nil, errors.New("email is required")
	}
	
	if req.Password == "" {
		return nil, errors.New("password is required")
	}

	if len(req.Password) < 8 {
		return nil, errors.New("password must be at least 8 characters long")
	}

	// Check if user already exists
	existingUser, err := s.getUserByEmail(req.Email)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to check existing user: %w", err)
	}
	if existingUser != nil {
		return nil, ErrUserExists
	}

	// Hash password
	hashedPassword, err := s.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create new user (default role is user)
	user := models.NewUser(req.Email, hashedPassword, models.UserRoleUser)

	// Insert user into database
	query := `
		INSERT INTO users (id, email, password_hash, role, rate_limit_rps, storage_quota_bytes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err = s.db.Exec(query,
		user.ID,
		user.Email,
		user.PasswordHash,
		user.Role,
		user.RateLimitRps,
		user.StorageQuotaBytes,
		user.CreatedAt,
		user.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Generate JWT token
	token, err := s.GenerateToken(user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &AuthResponse{
		User:  user,
		Token: token,
	}, nil
}

// SignUpAdmin creates a new admin user account
func (s *AuthService) SignUpAdmin(req *SignUpRequest) (*AuthResponse, error) {
	if req.Email == "" {
		return nil, errors.New("email is required")
	}
	
	if req.Password == "" {
		return nil, errors.New("password is required")
	}

	if len(req.Password) < 8 {
		return nil, errors.New("password must be at least 8 characters long")
	}

	// Check if user already exists
	existingUser, err := s.getUserByEmail(req.Email)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to check existing user: %w", err)
	}
	if existingUser != nil {
		return nil, ErrUserExists
	}

	// Hash password
	hashedPassword, err := s.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create new admin user
	user := models.NewUser(req.Email, hashedPassword, models.UserRoleAdmin)

	// Insert user into database
	query := `
		INSERT INTO users (id, email, password_hash, role, rate_limit_rps, storage_quota_bytes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err = s.db.Exec(query,
		user.ID,
		user.Email,
		user.PasswordHash,
		user.Role,
		user.RateLimitRps,
		user.StorageQuotaBytes,
		user.CreatedAt,
		user.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create admin user: %w", err)
	}

	// Generate JWT token
	token, err := s.GenerateToken(user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &AuthResponse{
		User:  user,
		Token: token,
	}, nil
}

// PromoteToAdmin promotes an existing user to admin role
func (s *AuthService) PromoteToAdmin(userID uuid.UUID) (*models.User, error) {
	// Get current user to check if exists
	user, err := s.GetUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Update user role to admin
	query := `
		UPDATE users 
		SET role = $1, updated_at = $2
		WHERE id = $3
	`

	_, err = s.db.Exec(query, models.UserRoleAdmin, time.Now(), userID)
	if err != nil {
		return nil, fmt.Errorf("failed to promote user to admin: %w", err)
	}

	// Return updated user
	user.Role = models.UserRoleAdmin
	user.UpdatedAt = time.Now()
	
	return user, nil
}

// Login authenticates user and returns token
func (s *AuthService) Login(req *LoginRequest) (*AuthResponse, error) {
	if req.Email == "" {
		return nil, errors.New("email is required")
	}
	
	if req.Password == "" {
		return nil, errors.New("password is required")
	}

	// Get user by email
	user, err := s.getUserByEmail(req.Email)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Google-only users cannot use password login
	if user.PasswordHash == "" && user.GoogleID.Valid {
		return nil, errors.New("this account uses Google sign-in, please login with Google")
	}

	// Check password
	if !s.CheckPassword(req.Password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	// Generate JWT token
	token, err := s.GenerateToken(user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &AuthResponse{
		User:  user,
		Token: token,
	}, nil
}

// GenerateToken generates a JWT token for the user
func (s *AuthService) GenerateToken(user *models.User) (string, error) {
	// Token expires in 24 hours
	expirationTime := time.Now().Add(24 * time.Hour)

	claims := &Claims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "securevault-backend",
			Subject:   user.ID.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// ValidateToken validates a JWT token and returns claims
func (s *AuthService) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	// Check if token is expired
	if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(time.Now()) {
		return nil, ErrTokenExpired
	}

	return claims, nil
}

// HashPassword hashes a password using bcrypt
func (s *AuthService) HashPassword(password string) (string, error) {
	// Use bcrypt with cost 12 for good security
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hashedBytes), nil
}

// CheckPassword verifies a password against its hash
func (s *AuthService) CheckPassword(password, hashedPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

// Authenticate validates user credentials and returns user info
func (s *AuthService) Authenticate(email, password string) (*models.User, error) {
	user, err := s.getUserByEmail(email)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if !s.CheckPassword(password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	return user, nil
}

// GetUserFromToken extracts and validates user from JWT token
func (s *AuthService) GetUserFromToken(tokenString string) (*models.User, error) {
	claims, err := s.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	// Get full user data from database
	user, err := s.getUserByID(claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user from token: %w", err)
	}

	return user, nil
}

// RefreshToken generates a new token for an existing valid token
func (s *AuthService) RefreshToken(tokenString string) (string, error) {
	claims, err := s.ValidateToken(tokenString)
	if err != nil {
		return "", err
	}

	// Get user from database to ensure they still exist
	user, err := s.getUserByID(claims.UserID)
	if err != nil {
		return "", fmt.Errorf("failed to get user for refresh: %w", err)
	}

	// Generate new token
	newToken, err := s.GenerateToken(user)
	if err != nil {
		return "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return newToken, nil
}

// GenerateRandomSecret generates a random JWT secret
func (s *AuthService) GenerateRandomSecret(length int) (string, error) {
	if length <= 0 {
		length = 32 // Default length
	}

	bytes := make([]byte, length)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random secret: %w", err)
	}

	return fmt.Sprintf("%x", bytes), nil
}

// ChangePassword changes user's password
func (s *AuthService) ChangePassword(userID uuid.UUID, oldPassword, newPassword string) error {
	if userID == uuid.Nil {
		return errors.New("user ID cannot be nil")
	}

	if len(newPassword) < 8 {
		return errors.New("new password must be at least 8 characters long")
	}

	// Get user
	user, err := s.getUserByID(userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Verify old password
	if !s.CheckPassword(oldPassword, user.PasswordHash) {
		return errors.New("old password is incorrect")
	}

	// Hash new password
	hashedPassword, err := s.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	// Update password in database
	query := `UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`
	_, err = s.db.Exec(query, hashedPassword, userID)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	return nil
}

// getUserByEmail is a helper method to get user by email
func (s *AuthService) getUserByEmail(email string) (*models.User, error) {
	var user models.User
	var passwordHash sql.NullString
	query := `
		SELECT id, email, password_hash, role, rate_limit_rps, storage_quota_bytes,
		       google_id, name, avatar_url, created_at, updated_at
		FROM users 
		WHERE email = $1
	`

	err := s.db.QueryRow(query, email).Scan(
		&user.ID,
		&user.Email,
		&passwordHash,
		&user.Role,
		&user.RateLimitRps,
		&user.StorageQuotaBytes,
		&user.GoogleID,
		&user.Name,
		&user.AvatarURL,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	if passwordHash.Valid {
		user.PasswordHash = passwordHash.String
	}

	return &user, nil
}

// getUserByID is a helper method to get user by ID
func (s *AuthService) getUserByID(userID uuid.UUID) (*models.User, error) {
	var user models.User
	var passwordHash sql.NullString
	query := `
		SELECT id, email, password_hash, role, rate_limit_rps, storage_quota_bytes,
		       google_id, name, avatar_url, created_at, updated_at
		FROM users 
		WHERE id = $1
	`

	err := s.db.QueryRow(query, userID).Scan(
		&user.ID,
		&user.Email,
		&passwordHash,
		&user.Role,
		&user.RateLimitRps,
		&user.StorageQuotaBytes,
		&user.GoogleID,
		&user.Name,
		&user.AvatarURL,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	if passwordHash.Valid {
		user.PasswordHash = passwordHash.String
	}

	return &user, nil
}

// GetUserByID is a public method to get user by ID
func (s *AuthService) GetUserByID(userID uuid.UUID) (*models.User, error) {
	return s.getUserByID(userID)
}

// IsAdmin checks if the user with given ID has admin role
func (s *AuthService) IsAdmin(userID uuid.UUID) (bool, error) {
	if userID == uuid.Nil {
		return false, fmt.Errorf("user ID cannot be nil")
	}

	var role models.UserRole
	query := `SELECT role FROM users WHERE id = $1`
	err := s.db.QueryRow(query, userID).Scan(&role)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, fmt.Errorf("user not found")
		}
		return false, fmt.Errorf("failed to get user role: %w", err)
	}

	return role == models.UserRoleAdmin, nil
}

// DeleteUser deletes a user from the database
func (s *AuthService) DeleteUser(userID uuid.UUID) error {
	if userID == uuid.Nil {
		return errors.New("user ID cannot be nil")
	}

	// Check if user exists
	_, err := s.GetUserByID(userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Delete user from database
	query := `DELETE FROM users WHERE id = $1`
	result, err := s.db.Exec(query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return errors.New("user not found")
	}

	return nil
}

// UpdatePasswordRequest represents a password update request
type UpdatePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// UpdatePassword updates a user's password after verifying the current password
func (s *AuthService) UpdatePassword(userID uuid.UUID, req *UpdatePasswordRequest) error {
	if userID == uuid.Nil {
		return errors.New("user ID cannot be nil")
	}

	if req.CurrentPassword == "" {
		return errors.New("current password is required")
	}

	if req.NewPassword == "" {
		return errors.New("new password is required")
	}

	if len(req.NewPassword) < 8 {
		return errors.New("new password must be at least 8 characters long")
	}

	// Get current user
	user, err := s.GetUserByID(userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Verify current password
	if !s.CheckPassword(req.CurrentPassword, user.PasswordHash) {
		return errors.New("current password is incorrect")
	}

	// Hash new password
	newPasswordHash, err := s.HashPassword(req.NewPassword)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	// Update password in database
	query := `UPDATE users SET password_hash = $1, updated_at = $2 WHERE id = $3`
	_, err = s.db.Exec(query, newPasswordHash, time.Now(), userID)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	return nil
}

// GoogleLoginRequest represents a Google OAuth login request
type GoogleLoginRequest struct {
	IDToken string `json:"id_token"`
}

// GoogleLogin verifies a Google ID token and creates or logs in the user
func (s *AuthService) GoogleLogin(req *GoogleLoginRequest) (*AuthResponse, error) {
	if req.IDToken == "" {
		return nil, errors.New("Google ID token is required")
	}

	// Get Google Client ID from environment
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	if clientID == "" {
		return nil, errors.New("Google OAuth is not configured")
	}

	// Verify the Google ID token
	payload, err := idtoken.Validate(context.Background(), req.IDToken, clientID)
	if err != nil {
		return nil, fmt.Errorf("invalid Google ID token: %w", err)
	}

	// Extract user info from the verified token payload
	email, _ := payload.Claims["email"].(string)
	name, _ := payload.Claims["name"].(string)
	picture, _ := payload.Claims["picture"].(string)
	googleID := payload.Subject // Google's unique user identifier

	if email == "" {
		return nil, errors.New("email not found in Google token")
	}

	// Check if user exists by Google ID first
	user, err := s.getUserByGoogleID(googleID)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to check existing Google user: %w", err)
	}

	if user != nil {
		// Existing Google user -- update profile info if changed
		s.updateGoogleUserProfile(user.ID, name, picture)

		token, err := s.GenerateToken(user)
		if err != nil {
			return nil, fmt.Errorf("failed to generate token: %w", err)
		}
		return &AuthResponse{User: user, Token: token}, nil
	}

	// Check if a user with the same email exists (email/password user)
	existingUser, err := s.getUserByEmail(email)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to check existing email user: %w", err)
	}

	if existingUser != nil {
		// Link Google account to existing email user
		err = s.linkGoogleAccount(existingUser.ID, googleID, name, picture)
		if err != nil {
			return nil, fmt.Errorf("failed to link Google account: %w", err)
		}

		// Refresh user data after linking
		existingUser, _ = s.getUserByID(existingUser.ID)

		token, err := s.GenerateToken(existingUser)
		if err != nil {
			return nil, fmt.Errorf("failed to generate token: %w", err)
		}
		return &AuthResponse{User: existingUser, Token: token}, nil
	}

	// New user -- create account from Google data
	newUser := models.NewGoogleUser(email, googleID, name, picture)

	query := `
		INSERT INTO users (id, email, role, rate_limit_rps, storage_quota_bytes, google_id, name, avatar_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err = s.db.Exec(query,
		newUser.ID,
		newUser.Email,
		newUser.Role,
		newUser.RateLimitRps,
		newUser.StorageQuotaBytes,
		newUser.GoogleID,
		newUser.Name,
		newUser.AvatarURL,
		newUser.CreatedAt,
		newUser.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Google user: %w", err)
	}

	token, err := s.GenerateToken(newUser)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &AuthResponse{User: newUser, Token: token}, nil
}

// getUserByGoogleID finds a user by their Google ID
func (s *AuthService) getUserByGoogleID(googleID string) (*models.User, error) {
	var user models.User
	var passwordHash sql.NullString
	query := `
		SELECT id, email, password_hash, role, rate_limit_rps, storage_quota_bytes,
		       google_id, name, avatar_url, created_at, updated_at
		FROM users
		WHERE google_id = $1
	`

	err := s.db.QueryRow(query, googleID).Scan(
		&user.ID,
		&user.Email,
		&passwordHash,
		&user.Role,
		&user.RateLimitRps,
		&user.StorageQuotaBytes,
		&user.GoogleID,
		&user.Name,
		&user.AvatarURL,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if passwordHash.Valid {
		user.PasswordHash = passwordHash.String
	}

	return &user, nil
}

// linkGoogleAccount links a Google account to an existing user
func (s *AuthService) linkGoogleAccount(userID uuid.UUID, googleID, name, picture string) error {
	query := `
		UPDATE users
		SET google_id = $1, name = COALESCE(NULLIF($2, ''), name), avatar_url = COALESCE(NULLIF($3, ''), avatar_url), updated_at = $4
		WHERE id = $5
	`
	_, err := s.db.Exec(query, googleID, name, picture, time.Now(), userID)
	return err
}

// updateGoogleUserProfile updates name and avatar from Google if they changed
func (s *AuthService) updateGoogleUserProfile(userID uuid.UUID, name, picture string) {
	query := `
		UPDATE users
		SET name = COALESCE(NULLIF($1, ''), name), avatar_url = COALESCE(NULLIF($2, ''), avatar_url), updated_at = $3
		WHERE id = $4
	`
	s.db.Exec(query, name, picture, time.Now(), userID)
}
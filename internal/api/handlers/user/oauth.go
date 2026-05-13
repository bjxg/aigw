package user

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

// stateEntry holds a state string with its expiration time.
type stateEntry struct {
	state     string
	expiresAt time.Time
}

// Handler holds OIDC runtime state.
type Handler struct {
	cfg          *config.Config
	provider     *oidc.Provider
	oauth2Config oauth2.Config
	verifier     *oidc.IDTokenVerifier

	states   map[string]stateEntry
	statesMu sync.RWMutex
}

// NewHandler creates a new user OIDC handler.
func NewHandler(cfg *config.Config) *Handler {
	h := &Handler{
		cfg:    cfg,
		states: make(map[string]stateEntry),
	}
	if cfg.OAuth.Enable && cfg.OAuth.ProviderURL != "" {
		if err := h.initProvider(context.Background()); err != nil {
			log.WithError(err).Warn("failed to initialize OIDC provider")
		}
	}
	return h
}

// initProvider initializes the OIDC provider and OAuth2 config.
func (h *Handler) initProvider(ctx context.Context) error {
	provider, err := oidc.NewProvider(ctx, h.cfg.OAuth.ProviderURL)
	if err != nil {
		return fmt.Errorf("oidc: create provider: %w", err)
	}
	h.provider = provider

	scopes := strings.Fields(h.cfg.OAuth.Scopes)
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}

	h.oauth2Config = oauth2.Config{
		ClientID:     h.cfg.OAuth.ClientID,
		ClientSecret: h.cfg.OAuth.ClientSecret,
		RedirectURL:  h.cfg.OAuth.RedirectURI,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}

	h.verifier = provider.Verifier(&oidc.Config{ClientID: h.cfg.OAuth.ClientID})
	return nil
}

// generateState creates a random state string.
func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// storeState saves a state string with a 5-minute TTL.
func (h *Handler) storeState(state string) {
	h.statesMu.Lock()
	defer h.statesMu.Unlock()
	// Clean up expired entries periodically
	now := time.Now()
	for k, v := range h.states {
		if now.After(v.expiresAt) {
			delete(h.states, k)
		}
	}
	h.states[state] = stateEntry{state: state, expiresAt: now.Add(5 * time.Minute)}
}

// verifyState checks if a state string exists and has not expired.
func (h *Handler) verifyState(state string) bool {
	h.statesMu.Lock()
	defer h.statesMu.Unlock()
	entry, ok := h.states[state]
	if !ok {
		return false
	}
	delete(h.states, state)
	return time.Now().Before(entry.expiresAt)
}

// Authorize handles GET /user/oauth/authorize.
func (h *Handler) Authorize(c *gin.Context) {
	if !h.cfg.OAuth.Enable {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "OIDC not enabled"})
		return
	}
	if h.provider == nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "OIDC provider not initialized"})
		return
	}

	state, err := generateState()
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state"})
		return
	}
	h.storeState(state)

	authURL := h.oauth2Config.AuthCodeURL(state)
	c.JSON(http.StatusOK, gin.H{
		"authorize_url": authURL,
		"state":         state,
	})
}

// Callback handles POST /user/oauth/callback.
func (h *Handler) Callback(c *gin.Context) {
	if !h.cfg.OAuth.Enable {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "OIDC not enabled"})
		return
	}
	if h.provider == nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "OIDC provider not initialized"})
		return
	}

	var body struct {
		Code  string `json:"code" binding:"required"`
		State string `json:"state" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid body: code and state are required"})
		return
	}

	if !h.verifyState(body.State) {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid or expired state"})
		return
	}

	ctx := context.Background()
	token, err := h.oauth2Config.Exchange(ctx, body.Code)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("oauth exchange failed: %v", err)})
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "no id_token in response"})
		return
	}

	idToken, err := h.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("id_token verification failed: %v", err)})
		return
	}

	var claims struct {
		Sub         string `json:"sub"`
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Email       string `json:"email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to parse claims: %v", err)})
		return
	}

	// Log all raw claims for debugging
	var rawClaims map[string]interface{}
	_ = idToken.Claims(&rawClaims)
	claimsJSON, _ := json.MarshalIndent(rawClaims, "", "  ")
	log.Infof("OIDC id_token claims:\n%s", claimsJSON)

	if claims.Sub == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "id_token missing sub claim"})
		return
	}

	// Match or create user
	user, err := usage.GormGetUserByUsername(claims.Sub)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	if user == nil {
		// Create new user
		role := strings.TrimSpace(h.cfg.OAuth.DefaultUserRole)
		if role == "" {
			role = usage.UserRoleUser
		}
		name := strings.TrimSpace(claims.DisplayName)
		if name == "" {
			name = strings.TrimSpace(claims.Name)
		}
		if name == "" {
			name = claims.Sub
		}
		email := strings.TrimSpace(claims.Email)
		var emailPtr *string
		if email != "" {
			emailPtr = &email
		}
		user = &usage.User{
			Name:     name,
			Username: &claims.Sub,
			Email:    emailPtr,
			Role:     role,
		}
		if err := usage.GormCreateUser(user); err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		// Update name/email if configured
		if h.cfg.OAuth.UpdateNameOnLogin {
			updates := make(map[string]interface{})
			name := strings.TrimSpace(claims.DisplayName)
			if name == "" {
				name = strings.TrimSpace(claims.Name)
			}
			if name != "" {
				updates["name"] = name
			}
			if strings.TrimSpace(claims.Email) != "" {
				updates["email"] = strings.TrimSpace(claims.Email)
			}
			if len(updates) > 0 {
				if err := usage.GormUpdateUser(user.ID, updates); err != nil {
					log.WithError(err).Warn("failed to update user on login")
				}
			}
		}
		// Update last seen
		updates := map[string]interface{}{"last_seen_at": now}
		if err := usage.GormUpdateUser(user.ID, updates); err != nil {
			log.WithError(err).Warn("failed to update last_seen_at")
		}
	}

	// Refresh user data after create/update
	user, err = usage.GormGetUserByID(user.ID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Create session
	sessionToken := uuid.New().String()
	expiresAt := now.Add(7 * 24 * time.Hour).Unix()
	session := &usage.OAuthSession{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		Provider:  h.cfg.OAuth.ProviderName,
		Token:     sessionToken,
		ExpiresAt: expiresAt,
		CreatedAt: now.Unix(),
		UpdatedAt: now.Unix(),
	}
	if err := usage.GormCreateOAuthSession(session); err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": sessionToken,
		"user":  user,
	})
}

// UserInfo handles GET /user/info.
func (h *Handler) UserInfo(c *gin.Context) {
	userID, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, ok := userID.(int64)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	user, err := usage.GormGetUserByID(id)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if user == nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}

// Logout handles POST /user/logout.
func (h *Handler) Logout(c *gin.Context) {
	token := c.GetHeader("Authorization")
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
		return
	}

	if err := usage.GormDeleteOAuthSessionByToken(token); err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

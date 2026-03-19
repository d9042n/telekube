package argocd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// authProvider applies authentication to an HTTP request.
type authProvider interface {
	Apply(req *http.Request) error
	// Refresh re-acquires credentials if they have expired. No-op for static tokens.
	Refresh(ctx context.Context) error
}

// TokenAuth applies a static Bearer token to every request.
type TokenAuth struct {
	token string
}

// NewTokenAuth creates a token-based auth provider.
func NewTokenAuth(token string) AuthProvider {
	return &TokenAuth{token: token}
}

// AuthProvider is the exported interface for auth providers.
type AuthProvider = authProvider

func (a *TokenAuth) Apply(req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+a.token)
	return nil
}

func (a *TokenAuth) Refresh(_ context.Context) error { return nil }

// OAuthAuth implements OAuth2 client-credentials flow with automatic token refresh.
type OAuthAuth struct {
	tokenURL     string
	clientID     string
	clientSecret string

	mu       sync.RWMutex
	token    string
	expiry   time.Time
	httpClient *http.Client
}

// NewOAuthAuth creates an OAuth2 client-credentials auth provider.
func NewOAuthAuth(tokenURL, clientID, clientSecret string, httpClient *http.Client) AuthProvider {
	return &OAuthAuth{
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   httpClient,
	}
}

func (a *OAuthAuth) Apply(req *http.Request) error {
	a.mu.RLock()
	token := a.token
	ok := time.Now().Before(a.expiry.Add(-30 * time.Second))
	a.mu.RUnlock()

	if !ok || token == "" {
		if err := a.Refresh(req.Context()); err != nil {
			return fmt.Errorf("refreshing oauth token: %w", err)
		}
		a.mu.RLock()
		token = a.token
		a.mu.RUnlock()
	}

	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// Refresh performs a client-credentials token request.
func (a *OAuthAuth) Refresh(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Double-check while holding the write lock.
	if time.Now().Before(a.expiry.Add(-30 * time.Second)) {
		return nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", a.clientID)
	form.Set("client_secret", a.clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.tokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token endpoint returned status %d", resp.StatusCode)
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decoding token response: %w", err)
	}

	a.token = result.AccessToken
	if result.ExpiresIn > 0 {
		a.expiry = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	} else {
		a.expiry = time.Now().Add(1 * time.Hour)
	}

	return nil
}

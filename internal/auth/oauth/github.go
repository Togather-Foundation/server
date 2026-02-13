package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHubConfig holds the OAuth configuration for GitHub authentication.
type GitHubConfig struct {
	ClientID     string
	ClientSecret string
	CallbackURL  string
	AllowedOrgs  []string // optional: restrict by org membership
}

// GitHubClient handles OAuth 2.0 authentication with GitHub.
type GitHubClient struct {
	config     GitHubConfig
	httpClient *http.Client
}

// GitHubUser represents a user authenticated via GitHub OAuth.
type GitHubUser struct {
	ID    int64  // GitHub user ID
	Login string // GitHub username
	Email string // Primary email address
	Name  string // Display name
}

// NewGitHubClient creates a new GitHub OAuth client with appropriate timeouts.
func NewGitHubClient(config GitHubConfig) *GitHubClient {
	return &GitHubClient{
		config: config,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GenerateAuthURL creates the GitHub authorization URL for initiating OAuth flow.
// The state parameter should be generated with GenerateState() and validated on callback.
func (c *GitHubClient) GenerateAuthURL(state string) string {
	params := url.Values{
		"client_id":    {c.config.ClientID},
		"redirect_uri": {c.config.CallbackURL},
		"scope":        {"user:email"},
		"state":        {state},
	}
	return "https://github.com/login/oauth/authorize?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for an access token.
// This is called after the user is redirected back from GitHub.
func (c *GitHubClient) ExchangeCode(ctx context.Context, code string) (string, error) {
	// Prepare token exchange request
	data := url.Values{
		"client_id":     {c.config.ClientID},
		"client_secret": {c.config.ClientSecret},
		"code":          {code},
		"redirect_uri":  {c.config.CallbackURL},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://github.com/login/oauth/access_token",
		strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResp.Error != "" {
		return "", fmt.Errorf("GitHub OAuth error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("no access token in response")
	}

	return tokenResp.AccessToken, nil
}

// FetchUserProfile retrieves the authenticated user's profile from GitHub.
// If the primary email is not public, it fetches from the /user/emails endpoint.
func (c *GitHubClient) FetchUserProfile(ctx context.Context, accessToken string) (*GitHubUser, error) {
	// Fetch user profile
	user, err := c.fetchUser(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	// If email is empty (user has private email), fetch from emails endpoint
	if user.Email == "" {
		email, err := c.fetchPrimaryEmail(ctx, accessToken)
		if err != nil {
			// Log but don't fail - email might not be available
			// In production, you might want to require email
			return user, nil
		}
		user.Email = email
	}

	return user, nil
}

// fetchUser fetches the basic user profile from GitHub.
func (c *GitHubClient) fetchUser(ctx context.Context, accessToken string) (*GitHubUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/user", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user profile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch user profile with status %d: %s", resp.StatusCode, string(body))
	}

	var userResp struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&userResp); err != nil {
		return nil, fmt.Errorf("failed to decode user response: %w", err)
	}

	return &GitHubUser{
		ID:    userResp.ID,
		Login: userResp.Login,
		Email: userResp.Email,
		Name:  userResp.Name,
	}, nil
}

// fetchPrimaryEmail fetches the user's primary email from the /user/emails endpoint.
// This is needed when users have their email set to private on GitHub.
func (c *GitHubClient) fetchPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/user/emails", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create emails request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch user emails: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to fetch user emails with status %d: %s", resp.StatusCode, string(body))
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", fmt.Errorf("failed to decode emails response: %w", err)
	}

	// Find primary verified email
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}

	// Fallback to first verified email
	for _, e := range emails {
		if e.Verified {
			return e.Email, nil
		}
	}

	return "", fmt.Errorf("no verified email found")
}

// GenerateState generates a cryptographically secure random state parameter
// for CSRF protection in OAuth flows. The state should be stored in a cookie
// or session and validated when GitHub redirects back.
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random state: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

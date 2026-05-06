package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	googleOAuth2 "golang.org/x/oauth2/google"
)

// OAuthUserInfo holds normalized profile data returned from any provider.
type OAuthUserInfo struct {
	ProviderID string
	Email      string
	Username   string
	AvatarURL  string
}

func GoogleOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     googleOAuth2.Endpoint,
	}
}

func GitHubOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("GITHUB_REDIRECT_URL"),
		Scopes:       []string{"user:email", "read:user"},
		Endpoint:     github.Endpoint,
	}
}

// GetGoogleUserInfo exchanges an OAuth code for user profile info.
func GetGoogleUserInfo(ctx context.Context, code string) (*OAuthUserInfo, error) {
	cfg := GoogleOAuthConfig()
	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("google token exchange: %w", err)
	}

	client := cfg.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return nil, fmt.Errorf("google userinfo fetch: %w", err)
	}
	defer resp.Body.Close()

	var raw struct {
		Sub     string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	return &OAuthUserInfo{
		ProviderID: raw.Sub,
		Email:      raw.Email,
		Username:   raw.Name,
		AvatarURL:  raw.Picture,
	}, nil
}

// GetGitHubUserInfo exchanges an OAuth code for GitHub user profile info.
func GetGitHubUserInfo(ctx context.Context, code string) (*OAuthUserInfo, error) {
	cfg := GitHubOAuthConfig()
	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("github token exchange: %w", err)
	}

	client := cfg.Client(ctx, token)

	// Fetch profile
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github user fetch: %w", err)
	}
	defer resp.Body.Close()

	var profile struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, err
	}

	// If email is private, fetch via /user/emails
	if profile.Email == "" {
		profile.Email, err = fetchGitHubPrimaryEmail(ctx, client)
		if err != nil {
			return nil, err
		}
	}

	return &OAuthUserInfo{
		ProviderID: fmt.Sprintf("%d", profile.ID),
		Email:      profile.Email,
		Username:   profile.Login,
		AvatarURL:  profile.AvatarURL,
	}, nil
}

func fetchGitHubPrimaryEmail(ctx context.Context, client *http.Client) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user/emails", nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var emails []struct {
		Email   string `json:"email"`
		Primary bool   `json:"primary"`
	}
	if err := json.Unmarshal(body, &emails); err != nil {
		return "", err
	}
	for _, e := range emails {
		if e.Primary {
			return e.Email, nil
		}
	}
	if len(emails) > 0 {
		return emails[0].Email, nil
	}
	return "", fmt.Errorf("no email found on GitHub account")
}

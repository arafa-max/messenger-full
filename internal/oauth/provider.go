package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

// UserInfo — нормализованные данные от любого провайдера
type UserInfo struct {
	ProviderID string
	Provider   string // "google" | "github"
	Email      string
	Name       string
	Avatar     string
}

type Provider interface {
	GetAuthURL(state string) string
	ExchangeCode(ctx context.Context, code string) (*UserInfo, error)
}

// ─── Google ───────────────────────────────────────────────────────────────────

type GoogleProvider struct {
	cfg *oauth2.Config
}

func NewGoogle(clientID, clientSecret, redirectURL string) *GoogleProvider {
	return &GoogleProvider{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		},
	}
}

func (g *GoogleProvider) GetAuthURL(state string) string {
	return g.cfg.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

func (g *GoogleProvider) ExchangeCode(ctx context.Context, code string) (*UserInfo, error) {
	token, err := g.cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("google token exchange: %w", err)
	}

	client := g.cfg.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return nil, fmt.Errorf("google userinfo: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var raw struct {
		Sub     string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("google userinfo parse: %w", err)
	}

	return &UserInfo{
		ProviderID: raw.Sub,
		Provider:   "google",
		Email:      raw.Email,
		Name:       raw.Name,
		Avatar:     raw.Picture,
	}, nil
}

// ─── GitHub ───────────────────────────────────────────────────────────────────

type GitHubProvider struct {
	cfg *oauth2.Config
}

func NewGitHub(clientID, clientSecret, redirectURL string) *GitHubProvider {
	return &GitHubProvider{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"read:user", "user:email"},
			Endpoint:     github.Endpoint,
		},
	}
}

func (g *GitHubProvider) GetAuthURL(state string) string {
	return g.cfg.AuthCodeURL(state)
}

func (g *GitHubProvider) ExchangeCode(ctx context.Context, code string) (*UserInfo, error) {
	token, err := g.cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("github token exchange: %w", err)
	}

	httpClient := g.cfg.Client(ctx, token)

	// Основной профиль
	userResp, err := httpClient.Get("https://api.github.com/user")
	if err != nil {
		return nil, fmt.Errorf("github user: %w", err)
	}
	defer userResp.Body.Close()
	userBody, _ := io.ReadAll(userResp.Body)

	var rawUser struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.Unmarshal(userBody, &rawUser); err != nil {
		return nil, fmt.Errorf("github user parse: %w", err)
	}

	// Email может быть скрыт — идём в /user/emails
	email := rawUser.Email
	if email == "" {
		if primary, err := fetchGitHubPrimaryEmail(httpClient); err == nil {
			email = primary
		}
	}

	name := rawUser.Name
	if name == "" {
		name = rawUser.Login
	}

	return &UserInfo{
		ProviderID: fmt.Sprintf("%d", rawUser.ID),
		Provider:   "github",
		Email:      email,
		Name:       name,
		Avatar:     rawUser.AvatarURL,
	}, nil
}

func fetchGitHubPrimaryEmail(client *http.Client) (string, error) {
	resp, err := client.Get("https://api.github.com/user/emails")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(body, &emails); err != nil {
		return "", err
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	return "", fmt.Errorf("no primary email found")
}
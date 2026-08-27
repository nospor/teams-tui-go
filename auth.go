package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// resolveTenant returns the Entra tenant for the auth endpoints:
// TENANT_ID from the environment/.env if set, otherwise "common".
// Single-tenant app registrations must set TENANT_ID — the /common
// endpoint rejects them with AADSTS50059.
func resolveTenant() string {
	_ = godotenv.Load()
	if t := os.Getenv("TENANT_ID"); t != "" {
		return t
	}
	return "common"
}

func deviceCodeURL() string {
	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/devicecode", resolveTenant())
}

func tokenURL() string {
	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", resolveTenant())
}

// DeviceCodeResponse is the response from the device code endpoint.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	Message         string `json:"message"`
}

// TokenResponse holds the OAuth2 token data.
type TokenResponse struct {
	AccessToken  string  `json:"access_token"`
	TokenType    string  `json:"token_type"`
	ExpiresIn    int     `json:"expires_in"`
	RefreshToken *string `json:"refresh_token,omitempty"`
	ExpiresAt    int64   `json:"expires_at"`
}

// tokenErrorResponse is returned when token polling is not yet complete or fails.
type tokenErrorResponse struct {
	Error string `json:"error"`
}

// tokenFilePath returns the path to the cached token file.
func tokenFilePath() (string, error) {
	dir, err := GetCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "token.json"), nil
}

// saveToken persists the token to disk.
func saveToken(token *TokenResponse) error {
	path, err := tokenFilePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal token: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// loadToken reads the token from disk.
// Returns nil, nil if the file does not exist.
func loadToken() (*TokenResponse, error) {
	path, err := tokenFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not read token file: %w", err)
	}
	var token TokenResponse
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("could not parse token file: %w", err)
	}
	// Back-fill ExpiresAt if it was not stored.
	if token.ExpiresAt == 0 && token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Unix() + int64(token.ExpiresIn)
	}
	return &token, nil
}

// StartDeviceFlow initiates the OAuth2 device code flow.
func StartDeviceFlow(clientID, scopes string) (*DeviceCodeResponse, error) {
	form := url.Values{
		"client_id": {clientID},
		"scope":     {scopes},
	}
	resp, err := http.PostForm(deviceCodeURL(), form)
	if err != nil {
		return nil, fmt.Errorf("device code request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code endpoint returned %d: %s", resp.StatusCode, body)
	}

	var dc DeviceCodeResponse
	if err := json.Unmarshal(body, &dc); err != nil {
		return nil, fmt.Errorf("could not parse device code response: %w", err)
	}
	return &dc, nil
}

// PollForToken polls the token endpoint until the user authenticates or the code expires.
func PollForToken(clientID, deviceCode string, interval int) (*TokenResponse, error) {
	if interval <= 0 {
		interval = 5
	}
	for {
		time.Sleep(time.Duration(interval) * time.Second)

		form := url.Values{
			"client_id":   {clientID},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"device_code": {deviceCode},
		}
		resp, err := http.PostForm(tokenURL(), form)
		if err != nil {
			return nil, fmt.Errorf("token poll request failed: %w", err)
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var token TokenResponse
			if err := json.Unmarshal(body, &token); err != nil {
				return nil, fmt.Errorf("could not parse token response: %w", err)
			}
			token.ExpiresAt = time.Now().Unix() + int64(token.ExpiresIn)
			if err := saveToken(&token); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not save token: %v\n", err)
			}
			return &token, nil
		}

		// Parse error response.
		var errResp tokenErrorResponse
		_ = json.Unmarshal(body, &errResp)
		switch errResp.Error {
		case "authorization_pending":
			// User hasn't completed auth yet — keep polling.
			continue
		case "authorization_declined":
			return nil, fmt.Errorf("authorization was declined by the user")
		case "expired_token":
			return nil, fmt.Errorf("device code expired; please restart and try again")
		default:
			return nil, fmt.Errorf("unexpected token error (%s): %s", errResp.Error, body)
		}
	}
}

// RefreshAccessToken uses the refresh token to obtain a new access token.
func RefreshAccessToken(clientID, refreshToken, scopes string) (*TokenResponse, error) {
	form := url.Values{
		"client_id":     {clientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"scope":         {scopes},
	}
	resp, err := http.PostForm(tokenURL(), form)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh endpoint returned %d: %s", resp.StatusCode, body)
	}

	var token TokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("could not parse refresh response: %w", err)
	}
	token.ExpiresAt = time.Now().Unix() + int64(token.ExpiresIn)
	// Preserve the old refresh token if none was returned.
	if token.RefreshToken == nil {
		token.RefreshToken = &refreshToken
	}
	if err := saveToken(&token); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save refreshed token: %v\n", err)
	}
	return &token, nil
}

// GetValidTokenSilent returns a valid access token from the cache, refreshing if necessary.
// Returns an error if the token is expired and cannot be refreshed.
func GetValidTokenSilent(clientID string) (string, error) {
	token, err := loadToken()
	if err != nil {
		return "", fmt.Errorf("could not load token: %w", err)
	}
	if token == nil {
		return "", fmt.Errorf("no cached token found")
	}

	// Check if token is still valid (with a 5-minute buffer).
	const bufferSeconds = 5 * 60
	if time.Now().Unix()+bufferSeconds < token.ExpiresAt {
		return token.AccessToken, nil
	}

	// Token expired — try to refresh.
	if token.RefreshToken == nil || *token.RefreshToken == "" {
		return "", fmt.Errorf("token expired and no refresh token available")
	}
	newToken, err := RefreshAccessToken(clientID, *token.RefreshToken, BuildScopes())
	if err != nil {
		return "", fmt.Errorf("token refresh failed: %w", err)
	}
	return newToken.AccessToken, nil
}

// GetAccessToken returns a valid access token, running the full device code flow if needed.
func GetAccessToken(clientID string) (string, error) {
	// Try silent first.
	if token, err := GetValidTokenSilent(clientID); err == nil {
		return token, nil
	}

	// Full device code flow.
	dc, err := StartDeviceFlow(clientID, BuildScopes())
	if err != nil {
		return "", fmt.Errorf("could not start device flow: %w", err)
	}

	// Print the user-facing instructions.
	fmt.Println(dc.Message)
	fmt.Printf("\nOpen: %s\nCode: %s\n\n", dc.VerificationURI, dc.UserCode)
	fmt.Println("Waiting for authentication...")

	token, err := PollForToken(clientID, dc.DeviceCode, dc.Interval)
	if err != nil {
		return "", fmt.Errorf("authentication failed: %w", err)
	}

	// Mask the token in output.
	masked := strings.Repeat("*", 8)
	_ = masked
	return token.AccessToken, nil
}

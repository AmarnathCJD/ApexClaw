package model

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func GetAnonymousToken() (string, error) {
	if t := os.Getenv("ZAI_TOKEN"); t != "" {
		return t, nil
	}
	resp, err := http.Get("https://chat.z.ai/api/v1/auths/")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth status %d", resp.StatusCode)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Token, nil
}

type TokenPair struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	IsFirstTime  bool   `json:"isFirstTime"`
}

type JWTClaims struct {
	IsFirstTime bool   `json:"isFirstTime"`
	SessionID   string `json:"sessionId"`
	jwt.RegisteredClaims
}

type RefreshTokenData struct {
	UserID      string
	ExpiresAt   time.Time
	IsFirstTime bool
}

type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string]*RefreshTokenData
}

var GlobalTokenStore = &TokenStore{
	tokens: make(map[string]*RefreshTokenData),
}

func (ts *TokenStore) StoreRefreshToken(tokenID string, data *RefreshTokenData) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.tokens[tokenID] = data
}

func (ts *TokenStore) GetRefreshToken(tokenID string) *RefreshTokenData {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	data, ok := ts.tokens[tokenID]
	if !ok || time.Now().After(data.ExpiresAt) {
		return nil
	}
	return data
}

func (ts *TokenStore) RevokeRefreshToken(tokenID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	delete(ts.tokens, tokenID)
}

func (ts *TokenStore) CleanupExpiredTokens() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	now := time.Now()
	for tokenID, data := range ts.tokens {
		if now.After(data.ExpiresAt) {
			delete(ts.tokens, tokenID)
		}
	}
}

func (ts *TokenStore) ClearAllTokens() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.tokens = make(map[string]*RefreshTokenData)
}

func GenerateSecureToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

type LoginRequest struct {
	Code string `json:"code"`
}

type ChangeCodeRequest struct {
	NewCode string `json:"newCode"`
}

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"apexclaw/core"
	"apexclaw/model"

	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
)

const (
	// JWT token expiration times
	accessTokenExpiry  = 1 * time.Hour
	refreshTokenExpiry = 7 * 24 * time.Hour

	// Cookie names
	refreshTokenCookie = "rt_token"
)

type ChatRequest struct {
	Message string `json:"message"`
	UserID  string `json:"user_id"`
}

func Start(addr string) error {
	model.GlobalTokenStore.ClearAllTokens()

	log.Printf("[Web] ***************************************")
	log.Printf("[Web] UI Login Code: %s", core.Cfg.WebLoginCode)
	log.Printf("[Web] ***************************************")

	fs := http.FileServer(http.Dir("frontend"))
	http.Handle("/", fs)

	http.HandleFunc("/api/auth/login", handleLogin)
	http.HandleFunc("/api/auth/refresh", handleRefresh)
	http.HandleFunc("/api/auth/change-code", authMiddleware(handleChangeCode))

	http.HandleFunc("/api/chat", authMiddleware(handleChat))
	http.HandleFunc("/api/settings", authMiddleware(handleSettings))

	log.Printf("[Web] listening on http://localhost%s", addr)
	return http.ListenAndServe(addr, nil)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req model.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Validate the code
	if req.Code != core.Cfg.WebLoginCode {
		http.Error(w, "Invalid login code", http.StatusUnauthorized)
		return
	}

	// Generate tokens
	accessToken, refreshTokenID, err := generateTokens()
	if err != nil {
		log.Printf("[AUTH] Token generation failed: %v", err)
		http.Error(w, "Token generation failed", http.StatusInternalServerError)
		return
	}

	// Store refresh token server-side
	model.GlobalTokenStore.StoreRefreshToken(refreshTokenID, &model.RefreshTokenData{
		UserID:      "web_user",
		ExpiresAt:   time.Now().Add(refreshTokenExpiry),
		IsFirstTime: core.Cfg.WebFirstLogin,
	})

	// Set refresh token as httpOnly cookie
	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookie,
		Value:    refreshTokenID,
		Path:     "/",
		Expires:  time.Now().Add(refreshTokenExpiry),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(model.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenID,
		IsFirstTime:  core.Cfg.WebFirstLogin,
	})
}

// handleRefresh validates the refresh token and returns a new access token
func handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get refresh token from cookie
	cookie, err := r.Cookie(refreshTokenCookie)
	if err != nil {
		http.Error(w, "No refresh token", http.StatusUnauthorized)
		return
	}

	refreshTokenID := cookie.Value

	// Validate refresh token
	tokenData := model.GlobalTokenStore.GetRefreshToken(refreshTokenID)
	if tokenData == nil {
		http.Error(w, "Invalid refresh token", http.StatusUnauthorized)
		return
	}

	// Generate new access token
	accessToken, err := generateAccessToken(tokenData.IsFirstTime)
	if err != nil {
		log.Printf("[AUTH] Access token generation failed: %v", err)
		http.Error(w, "Token generation failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"accessToken": accessToken,
	})
}

// handleChangeCode updates the login code (first-time setup)
func handleChangeCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req model.ChangeCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Validate new code format (6 digits)
	if !regexp.MustCompile(`^\d{6}$`).MatchString(req.NewCode) {
		http.Error(w, "Code must be 6 digits", http.StatusBadRequest)
		return
	}

	// Update config and .env
	oldCode := core.Cfg.WebLoginCode
	core.Cfg.WebLoginCode = req.NewCode

	if err := writeEnvValue("WEB_LOGIN_CODE", req.NewCode); err != nil {
		log.Printf("[AUTH] Failed to write WEB_LOGIN_CODE: %v", err)
		http.Error(w, "Failed to save code", http.StatusInternalServerError)
		return
	}
	if err := writeEnvValue("WEB_FIRST_LOGIN", "false"); err != nil {
		log.Printf("[AUTH] Failed to write WEB_FIRST_LOGIN: %v", err)
		http.Error(w, "Failed to save code", http.StatusInternalServerError)
		return
	}

	core.Cfg.WebFirstLogin = false

	log.Printf("[AUTH] Login code changed from %s to %s", oldCode, req.NewCode)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// ===== Middleware =====

// authMiddleware validates JWT token from Authorization header
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Missing authorization header", http.StatusUnauthorized)
			return
		}

		// Extract token from "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid authorization header", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]

		// Validate and parse JWT
		claims := &model.JWTClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(core.Cfg.WebJWTSecret), nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Check expiration explicitly
		if claims.ExpiresAt != nil && time.Now().After(claims.ExpiresAt.Time) {
			http.Error(w, "Token expired", http.StatusUnauthorized)
			return
		}

		// Store claims in context for downstream handlers
		ctx := context.WithValue(r.Context(), "jwt_claims", claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// ===== Protected Handlers =====

func handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	req.UserID = "web_" + core.Cfg.OwnerID

	if req.Message == "" {
		http.Error(w, "Empty message", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported by client", http.StatusInternalServerError)
		return
	}

	session := core.GetOrCreateAgentSession(req.UserID)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	_, err := session.RunStream(ctx, req.UserID, req.Message, func(chunk string) {
		if chunk == "" {
			return
		}
		if strings.HasPrefix(chunk, "__TOOL_CALL:") {
			toolName := strings.TrimPrefix(chunk, "__TOOL_CALL:")
			toolName = strings.TrimSuffix(toolName, "__\n")
			data, _ := json.Marshal(map[string]string{"type": "tool_call", "name": toolName})
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			flusher.Flush()
			return
		}
		if strings.HasPrefix(chunk, "__TOOL_RESULT:") {
			toolName := strings.TrimPrefix(chunk, "__TOOL_RESULT:")
			toolName = strings.TrimSuffix(toolName, "__\n")
			data, _ := json.Marshal(map[string]string{"type": "tool_result", "name": toolName})
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			flusher.Flush()
			return
		}

		data, _ := json.Marshal(map[string]string{"type": "chunk", "chunk": chunk})
		fmt.Fprintf(w, "data: %s\n\n", string(data))
		flusher.Flush()
	})

	if err != nil {
		data, _ := json.Marshal(map[string]interface{}{"type": "error", "error": err.Error()})
		fmt.Fprintf(w, "data: %s\n\n", string(data))
	} else {
		data, _ := json.Marshal(map[string]interface{}{"type": "done", "done": true})
		fmt.Fprintf(w, "data: %s\n\n", string(data))
	}
	flusher.Flush()
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		envMap, err := godotenv.Read()
		if err != nil {
			http.Error(w, "Could not read .env file", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(envMap)
		return
	}

	if r.Method == http.MethodPost {
		var newSettings map[string]string
		if err := json.NewDecoder(r.Body).Decode(&newSettings); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		envMap, err := godotenv.Read()
		if err != nil {
			envMap = make(map[string]string)
		}
		for k, v := range newSettings {
			envMap[k] = v
		}

		if err := godotenv.Write(envMap, ".env"); err != nil {
			http.Error(w, "Failed to write .env", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// ===== Token Generation =====

// generateTokens creates both access and refresh JWT tokens
func generateTokens() (accessToken, refreshTokenID string, err error) {
	// Generate access token
	accessToken, err = generateAccessToken(core.Cfg.WebFirstLogin)
	if err != nil {
		return "", "", err
	}

	// Generate refresh token ID (stored server-side)
	refreshTokenID, err = model.GenerateSecureToken()
	if err != nil {
		return "", "", err
	}

	return accessToken, refreshTokenID, nil
}

// generateAccessToken creates a JWT access token
func generateAccessToken(isFirstTime bool) (string, error) {
	sessionID := fmt.Sprintf("session_%d_%d", time.Now().Unix(), time.Now().Nanosecond())
	claims := &model.JWTClaims{
		IsFirstTime: isFirstTime,
		SessionID:   sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(accessTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(core.Cfg.WebJWTSecret))
}

// writeEnvValue updates a single key-value pair in .env file with proper quoting
func writeEnvValue(key, value string) error {
	envMap, _ := godotenv.Read()
	if envMap == nil {
		envMap = make(map[string]string)
	}
	envMap[key] = value
	return godotenv.Write(envMap, ".env")
}

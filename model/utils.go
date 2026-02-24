package model

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	feVersion   string
	versionLock sync.RWMutex
)

func GetFeVersion() string {
	versionLock.RLock()
	defer versionLock.RUnlock()
	return feVersion
}

func StartVersionUpdater() {
	fetchFeVersion()
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			fetchFeVersion()
		}
	}()
}

func GenerateSignature(userID, requestID, userContent string, timestamp int64) string {
	requestInfo := fmt.Sprintf("requestId,%s,timestamp,%d,user_id,%s", requestID, timestamp, userID)
	contentBase64 := base64.StdEncoding.EncodeToString([]byte(userContent))
	signData := fmt.Sprintf("%s|%s|%d", requestInfo, contentBase64, timestamp)

	period := timestamp / (5 * 60 * 1000)
	firstHmac := hmacSha256Hex([]byte("key-@@@@)))()((9))-xxxx&&&%%%%%"), fmt.Sprintf("%d", period))
	return hmacSha256Hex([]byte(firstHmac), signData)
}

func hmacSha256Hex(key []byte, data string) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func fetchFeVersion() {
	resp, err := http.Get("https://chat.z.ai/")
	if err != nil {
		log.Printf("[MODEL] fe version fetch error: %v", err)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	re := regexp.MustCompile(`prod-fe-[\.\d]+`)
	if match := re.FindString(string(body)); match != "" {
		versionLock.Lock()
		feVersion = match
		versionLock.Unlock()
		log.Printf("[MODEL] fe version: %s", match)
	}
}

type JWTPayload struct {
	ID string `json:"id"`
}

func DecodeJWTPayload(token string) (*JWTPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, nil
	}
	payload := parts[1]
	if padding := 4 - len(payload)%4; padding != 4 {
		payload += strings.Repeat("=", padding)
	}
	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, err
		}
	}
	var result JWTPayload
	if err := json.Unmarshal(decoded, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

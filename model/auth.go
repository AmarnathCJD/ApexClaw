package model

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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

package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var Translate = &ToolDef{
	Name:        "translate",
	Description: "Translate text between languages using MyMemory API. Supports 60+ languages. Use language codes like 'en', 'hi', 'es', 'fr', 'de', 'ar', 'zh', 'ja', 'ko', 'ru', 'pt', 'ml', 'ta', 'te', 'bn'.",
	Args: []ToolArg{
		{Name: "text", Description: "Text to translate", Required: true},
		{Name: "to", Description: "Target language code (e.g. 'hi' for Hindi, 'es' for Spanish, 'fr' for French)", Required: true},
		{Name: "from", Description: "Source language code (default 'en' for English). Use 'auto' to auto-detect.", Required: false},
	},
	Execute: func(args map[string]string) string {
		text := strings.TrimSpace(args["text"])
		to := strings.TrimSpace(args["to"])
		from := strings.TrimSpace(args["from"])

		if text == "" {
			return "Error: text is required"
		}
		if to == "" {
			return "Error: to language is required"
		}
		if from == "" || strings.EqualFold(from, "auto") {
			from = "en"
		}

		langPair := from + "|" + to
		apiURL := fmt.Sprintf(
			"https://api.mymemory.translated.net/get?q=%s&langpair=%s",
			url.QueryEscape(text),
			url.QueryEscape(langPair),
		)

		client := &http.Client{Timeout: 15 * time.Second}
		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		req.Header.Set("User-Agent", "ApexClaw/1.0")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Translation error: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		var result struct {
			ResponseData struct {
				TranslatedText string  `json:"translatedText"`
				Match          float64 `json:"match"`
			} `json:"responseData"`
			ResponseStatus  int    `json:"responseStatus"`
			ResponseDetails string `json:"responseDetails"`
			Matches         []struct {
				Translation string  `json:"translation"`
				Quality     float64 `json:"quality"`
			} `json:"matches"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Sprintf("Parse error: %v", err)
		}
		if result.ResponseStatus != 200 {
			return fmt.Sprintf("Translation failed (%d): %s", result.ResponseStatus, result.ResponseDetails)
		}

		translated := result.ResponseData.TranslatedText
		if translated == "" {
			return "Translation returned empty result"
		}

		return fmt.Sprintf("[%s → %s]\n%s", from, to, translated)
	},
}

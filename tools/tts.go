package tools

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var TextToSpeech = &ToolDef{
	Name: "text_to_speech",
	Description: "Convert text to speech and send the audio to Telegram. Uses Google TTS (free, no API key needed). " +
		"Supports many languages. Sends an audio file directly to the current chat.",
	Secure: true,
	Args: []ToolArg{
		{Name: "text", Description: "The text to convert to speech (max ~200 chars for best quality)", Required: true},
		{Name: "lang", Description: "Language code (e.g. 'en', 'hi', 'ta', 'te', 'ml', 'fr', 'es', 'de', 'ja', 'ko'). Default: 'en'", Required: false},
		{Name: "slow", Description: "Set to 'true' for slower speech (useful for language learning)", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		text := strings.TrimSpace(args["text"])
		if text == "" {
			return "Error: text is required"
		}
		lang := strings.TrimSpace(args["lang"])
		if lang == "" {
			lang = "en"
		}
		slow := strings.EqualFold(strings.TrimSpace(args["slow"]), "true")
		slowParam := "0"
		if slow {
			slowParam = "1"
		}

		chunks := chunkText(text, 100)
		var audioData []byte
		for _, chunk := range chunks {
			ttsURL := fmt.Sprintf(
				"https://translate.google.com/translate_tts?ie=UTF-8&q=%s&tl=%s&slow=%s&client=gtx",
				url.QueryEscape(chunk), url.QueryEscape(lang), slowParam,
			)
			req, err := http.NewRequest("GET", ttsURL, nil)
			if err != nil {
				return fmt.Sprintf("Error building TTS request: %v", err)
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			req.Header.Set("Referer", "https://translate.google.com/")

			client := &http.Client{Timeout: 20 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Sprintf("Error fetching TTS audio: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				return fmt.Sprintf("TTS service returned HTTP %d", resp.StatusCode)
			}
			chunk, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Sprintf("Error reading TTS response: %v", err)
			}
			audioData = append(audioData, chunk...)
		}

		tmpFile, err := os.CreateTemp("", "tts-*.mp3")
		if err != nil {
			return fmt.Sprintf("Error creating temp file: %v", err)
		}
		tmpPath := tmpFile.Name()
		defer func() {
			tmpFile.Close()
			os.Remove(tmpPath)
		}()
		if _, err := tmpFile.Write(audioData); err != nil {
			return fmt.Sprintf("Error writing audio: %v", err)
		}
		tmpFile.Close()

		var chatID int64
		if GetTelegramContextFn != nil {
			if ctx := GetTelegramContextFn(userID); ctx != nil {
				if v, ok := ctx["telegram_id"]; ok {
					chatID = v.(int64)
				}
			}
		}
		if chatID == 0 {
			return fmt.Sprintf("Audio saved to %s (no Telegram context to send to)", tmpPath)
		}
		if SendTGFileFn == nil {
			return "Error: Telegram file sender not initialized"
		}

		caption := fmt.Sprintf("🔊 %s [%s]", truncateTTS(text, 60), strings.ToUpper(lang))
		if result := SendTGFileFn(chatID, tmpPath, caption); result != "" {
			return fmt.Sprintf("Error sending audio: %s", result)
		}

		niceName := filepath.Join(os.TempDir(), "speech.mp3")
		_ = os.Rename(tmpPath, niceName)

		return fmt.Sprintf("🔊 Sent TTS audio (%s, %s)", lang, truncateTTS(text, 40))
	},
}

func chunkText(text string, maxLen int) []string {
	words := strings.Fields(text)
	var chunks []string
	var current strings.Builder
	for _, w := range words {
		if current.Len()+len(w)+1 > maxLen && current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
		}
		if current.Len() > 0 {
			current.WriteByte(' ')
		}
		current.WriteString(w)
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

func truncateTTS(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

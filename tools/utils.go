package tools

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var Datetime = &ToolDef{
	Name:        "datetime",
	Description: "Get the current date, time, day of week, and timezone",
	Args:        []ToolArg{},
	Execute: func(args map[string]string) string {
		now := time.Now()
		return fmt.Sprintf(
			"Date: %s\nTime: %s\nDay: %s\nTimezone: %s\nUnix: %d",
			now.Format("2006-01-02"),
			now.Format("15:04:05"),
			now.Weekday().String(),
			now.Format("MST"),
			now.Unix(),
		)
	},
}

var Timer = &ToolDef{
	Name:        "timer",
	Description: "Wait for a specified number of seconds (max 30)",
	Args: []ToolArg{
		{Name: "seconds", Description: "How many seconds to wait (max 30)", Required: true},
	},
	Execute: func(args map[string]string) string {
		secStr := args["seconds"]
		if secStr == "" {
			return "Error: seconds is required"
		}
		var sec int
		fmt.Sscanf(secStr, "%d", &sec)
		if sec <= 0 {
			return "Error: seconds must be positive"
		}
		if sec > 30 {
			sec = 30
		}
		time.Sleep(time.Duration(sec) * time.Second)
		return fmt.Sprintf("Waited %d second(s).", sec)
	},
}

var Echo = &ToolDef{
	Name:        "echo",
	Description: "Echo back the given text — useful for testing",
	Args: []ToolArg{
		{Name: "text", Description: "Text to echo back", Required: true},
	},
	Execute: func(args map[string]string) string {
		return strings.TrimSpace(args["text"])
	},
}

var QRCodeGenerate = &ToolDef{
	Name:        "qrcode_generate",
	Description: "Generate a QR code containing text or URL, returns local file path of image",
	Args: []ToolArg{
		{Name: "data", Description: "Text or URL to encode", Required: true},
	},
	Execute: func(args map[string]string) string {
		data := args["data"]
		if data == "" {
			return "Error: data required"
		}

		u := "https://api.qrserver.com/v1/create-qr-code/?size=500x500&data=" + url.QueryEscape(data)
		resp, err := http.Get(u)
		if err != nil {
			return fmt.Sprintf("Error generating QR: %v", err)
		}
		defer resp.Body.Close()

		f, err := os.CreateTemp("", "qrcode-*.png")
		if err != nil {
			return err.Error()
		}
		defer f.Close()
		io.Copy(f, resp.Body)

		return fmt.Sprintf("QR code generated: %s (You can send this file using tg_send_photo)", f.Name())
	},
}

var URLShorten = &ToolDef{
	Name:        "url_shorten",
	Description: "Shorten a long URL using Is.Gd API",
	Args: []ToolArg{
		{Name: "url", Description: "URL to shorten", Required: true},
	},
	Execute: func(args map[string]string) string {
		longURL := strings.TrimSpace(args["url"])
		if longURL == "" {
			return "Error: url required"
		}
		reqURL := "https://is.gd/create.php?format=simple&url=" + url.QueryEscape(longURL)

		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Get(reqURL)
		if err != nil {
			return fmt.Sprintf("Error shortening URL: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		res := string(body)
		if strings.HasPrefix(res, "Error") {
			return res
		}
		return "Shortened URL: " + res
	},
}

var UUIDGenerate = &ToolDef{
	Name:        "uuid_generate",
	Description: "Generate a random UUID v4",
	Args:        []ToolArg{},
	Execute: func(args map[string]string) string {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		b[6] = (b[6] & 0x0f) | 0x40 // Version 4
		b[8] = (b[8] & 0x3f) | 0x80 // Variant is 10
		return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	},
}

var PasswordGenerate = &ToolDef{
	Name:        "password_generate",
	Description: "Generate a secure random password",
	Args: []ToolArg{
		{Name: "length", Description: "Length of password (default 16, max 128)", Required: false},
	},
	Execute: func(args map[string]string) string {
		length := 16
		if l := args["length"]; l != "" {
			fmt.Sscanf(l, "%d", &length)
		}
		if length < 4 {
			length = 4
		} else if length > 128 {
			length = 128
		}
		const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+[]{}|;:,.<>?"
		var result strings.Builder
		for i := 0; i < length; i++ {
			n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
			result.WriteByte(charset[n.Int64()])
		}
		return result.String()
	},
}

var JokeFetch = &ToolDef{
	Name:        "joke_fetch",
	Description: "Fetch a random joke",
	Args:        []ToolArg{},
	Execute: func(args map[string]string) string {
		resp, err := http.Get("https://official-joke-api.appspot.com/random_joke")
		if err != nil {
			return "Error fetching joke"
		}
		defer resp.Body.Close()
		var joke struct {
			Setup     string `json:"setup"`
			Punchline string `json:"punchline"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&joke); err != nil {
			return "Error decoding joke"
		}
		return joke.Setup + "\n\n" + joke.Punchline
	},
}

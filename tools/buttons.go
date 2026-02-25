package tools

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
)

var SendTGMessageWithButtonsFn func(chatID int64, text string, kb *telegram.ReplyInlineMarkup) string

type ButtonSpec struct {
	Text  string `json:"text"`
	Type  string `json:"type"`  // "data" or "url"
	Data  string `json:"data"`  // callback data for type=data
	URL   string `json:"url"`   // url for type=url
	Style string `json:"style"` // "danger", "success", "primary"
}

type ButtonRowSpec struct {
	Buttons []ButtonSpec `json:"buttons"`
}

type ButtonsSpec struct {
	Rows []ButtonRowSpec `json:"rows"`
}

var TGSendMessageWithButtons = &ToolDef{
	Name:        "tg_send_message_buttons",
	Description: "Send Telegram message with inline buttons. Use this tool when user wants buttons, colors, interactions, or confirmations. Buttons parameter should be base64-encoded JSON (e.g., eyJyb3dzIjpbeyJidXR0b25zIjpbeyJ0ZXh0IjoiWWVzIiwidHlwZSI6ImRhdGEiLCJkYXRhIjoieWVzIiwic3R5bGUiOiJzdWNjZXNzIn1dfV19). Decoded format: {\"rows\":[{\"buttons\":[{\"text\":\"Yes\",\"type\":\"data\",\"data\":\"yes\",\"style\":\"success\"}]}]}. Styles: success(green), danger(red), primary(blue). Type: data(callback) or url(link).",
	Secure:      true,
	Args: []ToolArg{
		{Name: "text", Description: "Message text to send", Required: true},
		{Name: "buttons", Description: "Buttons as BASE64-ENCODED JSON. INCLUDE THIS when user asks for buttons/colors/clicks/interactions/confirmations. Example JSON to encode: {\"rows\":[{\"buttons\":[{\"text\":\"Green\",\"type\":\"data\",\"data\":\"click_green\",\"style\":\"success\"}]}]}", Required: false},
		{Name: "target", Description: "Chat ID or 'me'. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		text := strings.TrimSpace(args["text"])
		buttonsJSON := strings.TrimSpace(args["buttons"])
		target := strings.TrimSpace(args["target"])

		if text == "" {
			return "Error: text is required"
		}

		var chatID int64
		if GetTelegramContextFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				if v, ok := ctx["telegram_id"]; ok {
					chatID = v.(int64)
				}
			}
		}

		if target != "" {
			id, err := parseTarget(target, userID)
			if err != nil {
				return fmt.Sprintf("Error: %v", err)
			}
			chatID = id
		}

		if chatID == 0 {
			return "Error: no current chat context"
		}

		if SendTGMessageWithButtonsFn == nil {
			return "Error: button sender not initialized"
		}

		var kb *telegram.ReplyInlineMarkup
		if buttonsJSON != "" {
			kb = parseButtons(buttonsJSON)
			if kb == nil {
				return "Error: failed to parse buttons JSON"
			}
		}

		return SendTGMessageWithButtonsFn(chatID, text, kb)
	},
}

func parseButtons(buttonsB64 string) *telegram.ReplyInlineMarkup {
	// Decode from base64
	jsonBytes, err := base64.StdEncoding.DecodeString(buttonsB64)
	if err != nil {
		return nil
	}

	var spec ButtonsSpec
	if err := json.Unmarshal(jsonBytes, &spec); err != nil {
		return nil
	}
	return parseButtonsSpec(&spec)
}

// parseButtonsSpec converts ButtonsSpec to telegram.ReplyInlineMarkup
func parseButtonsSpec(spec *ButtonsSpec) *telegram.ReplyInlineMarkup {
	kb := telegram.NewKeyboard()

	for _, rowSpec := range spec.Rows {
		var rowButtons []telegram.KeyboardButton

		for _, btnSpec := range rowSpec.Buttons {
			var btn telegram.KeyboardButton

			// Create button based on type
			switch btnSpec.Type {
			case "url":
				b := telegram.Button.URL(btnSpec.Text, btnSpec.URL)
				switch btnSpec.Style {
				case "success":
					b.Success()
				case "danger":
					b.Danger()
				default:
					b.Primary()
				}
				btn = b
			case "data":
				fallthrough
			default:
				b := telegram.Button.Data(btnSpec.Text, btnSpec.Data)
				switch btnSpec.Style {
				case "success":
					b.Success()
				case "danger":
					b.Danger()
				default:
					b.Primary()
				}
				btn = b
			}

			rowButtons = append(rowButtons, btn)
		}

		if len(rowButtons) > 0 {
			kb.AddRow(rowButtons...)
		}
	}

	return kb.Build()
}

func parseTarget(target, userID string) (int64, error) {
	switch target {
	case "", "current":
		if GetTelegramContextFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				if v, ok := ctx["telegram_id"]; ok {
					return v.(int64), nil
				}
			}
		}
		return 0, fmt.Errorf("no current chat context")
	case "me":
		if GetTelegramContextFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				if v, ok := ctx["owner_id"]; ok {
					ownerStr := v.(string)
					var id int64
					_, err := fmt.Sscanf(ownerStr, "%d", &id)
					if err == nil {
						return id, nil
					}
				}
			}
		}
		return 0, fmt.Errorf("could not resolve 'me'")
	default:
		var id int64
		_, err := fmt.Sscanf(target, "%d", &id)
		if err != nil {
			return 0, fmt.Errorf("target must be numeric or 'me'")
		}
		return id, nil
	}
}

package tools

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var Wikipedia = &ToolDef{
	Name:        "wikipedia",
	Description: "Search Wikipedia and get a summary of any topic or article",
	Args: []ToolArg{
		{Name: "query", Description: "Topic or article title to look up", Required: true},
		{Name: "lang", Description: "Wikipedia language code (default: en)", Required: false},
	},
	Execute: func(args map[string]string) string {
		query := strings.TrimSpace(args["query"])
		if query == "" {
			return "Error: query is required"
		}
		lang := strings.TrimSpace(args["lang"])
		if lang == "" {
			lang = "en"
		}

		client := &http.Client{Timeout: 15 * time.Second}

		searchURL := fmt.Sprintf(
			"https://%s.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&format=json&srlimit=1",
			lang, url.QueryEscape(query),
		)
		req, _ := http.NewRequest("GET", searchURL, nil)
		req.Header.Set("User-Agent", "ApexClaw/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error searching Wikipedia: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		var searchResult struct {
			Query struct {
				Search []struct {
					Title string `json:"title"`
				} `json:"search"`
			} `json:"query"`
		}
		if err := json.Unmarshal(body, &searchResult); err != nil || len(searchResult.Query.Search) == 0 {
			return fmt.Sprintf("No Wikipedia article found for: %s", query)
		}
		title := searchResult.Query.Search[0].Title

		summaryURL := fmt.Sprintf(
			"https://%s.wikipedia.org/api/rest_v1/page/summary/%s",
			lang, url.PathEscape(title),
		)
		req2, _ := http.NewRequest("GET", summaryURL, nil)
		req2.Header.Set("User-Agent", "ApexClaw/1.0")
		resp2, err := client.Do(req2)
		if err != nil {
			return fmt.Sprintf("Error fetching article: %v", err)
		}
		defer resp2.Body.Close()
		body2, _ := io.ReadAll(resp2.Body)

		var summary struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Extract     string `json:"extract"`
			ContentURLs struct {
				Desktop struct {
					Page string `json:"page"`
				} `json:"desktop"`
			} `json:"content_urls"`
		}
		if err := json.Unmarshal(body2, &summary); err != nil {
			return fmt.Sprintf("Error parsing article: %v", err)
		}

		extract := strings.TrimSpace(summary.Extract)
		if len(extract) > 2000 {
			extract = extract[:2000] + "..."
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Wikipedia: %s\n", summary.Title))
		if summary.Description != "" {
			sb.WriteString(fmt.Sprintf("(%s)\n", summary.Description))
		}
		sb.WriteString(strings.Repeat("─", 36) + "\n")
		sb.WriteString(extract + "\n")
		if summary.ContentURLs.Desktop.Page != "" {
			sb.WriteString(fmt.Sprintf("\nSource: %s", summary.ContentURLs.Desktop.Page))
		}
		return sb.String()
	},
}

var CurrencyConvert = &ToolDef{
	Name:        "currency_convert",
	Description: "Convert an amount between currencies using live exchange rates (e.g. USD to EUR, INR to GBP)",
	Args: []ToolArg{
		{Name: "amount", Description: "Amount to convert (default: 1)", Required: false},
		{Name: "from", Description: "Source currency code (e.g. USD, EUR, INR, GBP)", Required: true},
		{Name: "to", Description: "Target currency code(s), comma-separated (e.g. EUR or EUR,GBP,JPY)", Required: true},
	},
	Execute: func(args map[string]string) string {
		from := strings.ToUpper(strings.TrimSpace(args["from"]))
		to := strings.ToUpper(strings.TrimSpace(args["to"]))
		if from == "" || to == "" {
			return "Error: from and to currency codes are required"
		}

		amount := 1.0
		if a := args["amount"]; a != "" {
			fmt.Sscanf(a, "%f", &amount)
		}

		toClean := strings.ReplaceAll(to, " ", "")
		apiURL := fmt.Sprintf("https://api.frankfurter.app/latest?from=%s&to=%s", from, toClean)

		client := &http.Client{Timeout: 10 * time.Second}
		req, _ := http.NewRequest("GET", apiURL, nil)
		req.Header.Set("User-Agent", "ApexClaw/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error fetching rates: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		var result struct {
			Base  string             `json:"base"`
			Date  string             `json:"date"`
			Rates map[string]float64 `json:"rates"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Sprintf("Error parsing rates: %v", err)
		}
		if len(result.Rates) == 0 {
			return fmt.Sprintf("No rates found. Check currency codes (from=%s, to=%s). Use standard ISO 4217 codes.", from, to)
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Exchange rates as of %s\n", result.Date))
		sb.WriteString(strings.Repeat("─", 30) + "\n")
		sb.WriteString(fmt.Sprintf("%.2f %s =\n", amount, from))
		for currency, rate := range result.Rates {
			converted := amount * rate
			sb.WriteString(fmt.Sprintf("  %10.4f  %s\n", converted, currency))
		}
		return strings.TrimRight(sb.String(), "\n")
	},
}

var HashText = &ToolDef{
	Name:        "hash_text",
	Description: "Generate a cryptographic hash of text — supports MD5, SHA1, SHA256, SHA512",
	Args: []ToolArg{
		{Name: "text", Description: "Text to hash", Required: true},
		{Name: "algorithm", Description: "Hash algorithm: md5, sha1, sha256, sha512 (default: sha256)", Required: false},
	},
	Execute: func(args map[string]string) string {
		text := args["text"]
		if text == "" {
			return "Error: text is required"
		}
		algo := strings.ToLower(strings.TrimSpace(args["algorithm"]))
		if algo == "" {
			algo = "sha256"
		}

		data := []byte(text)
		var hashHex string
		switch algo {
		case "md5":
			h := md5.Sum(data)
			hashHex = hex.EncodeToString(h[:])
		case "sha1":
			h := sha1.Sum(data)
			hashHex = hex.EncodeToString(h[:])
		case "sha256":
			h := sha256.Sum256(data)
			hashHex = hex.EncodeToString(h[:])
		case "sha512":
			h := sha512.Sum512(data)
			hashHex = hex.EncodeToString(h[:])
		default:
			return fmt.Sprintf("Error: unknown algorithm %q — use md5, sha1, sha256, or sha512", algo)
		}

		return fmt.Sprintf("Algorithm: %s\nInput:     %s\nHash:      %s", strings.ToUpper(algo), text, hashHex)
	},
}

var EncodeDecode = &ToolDef{
	Name:        "encode_decode",
	Description: "Encode or decode text: base64, URL encoding, or hex",
	Args: []ToolArg{
		{Name: "text", Description: "Text to encode or decode", Required: true},
		{Name: "operation", Description: "Operation: base64_encode, base64_decode, url_encode, url_decode, hex_encode, hex_decode", Required: true},
	},
	Execute: func(args map[string]string) string {
		text := args["text"]
		if text == "" {
			return "Error: text is required"
		}
		op := strings.ToLower(strings.TrimSpace(args["operation"]))
		if op == "" {
			return "Error: operation is required (base64_encode, base64_decode, url_encode, url_decode, hex_encode, hex_decode)"
		}

		switch op {
		case "base64_encode":
			return base64.StdEncoding.EncodeToString([]byte(text))

		case "base64_decode":
			decoded, err := base64.StdEncoding.DecodeString(text)
			if err != nil {

				decoded, err = base64.URLEncoding.DecodeString(text)
				if err != nil {

					decoded, err = base64.RawStdEncoding.DecodeString(text)
					if err != nil {
						return fmt.Sprintf("Error decoding base64: %v", err)
					}
				}
			}
			return string(decoded)

		case "url_encode":
			return url.QueryEscape(text)

		case "url_decode":
			decoded, err := url.QueryUnescape(text)
			if err != nil {
				return fmt.Sprintf("Error decoding URL: %v", err)
			}
			return decoded

		case "hex_encode":
			return hex.EncodeToString([]byte(text))

		case "hex_decode":
			decoded, err := hex.DecodeString(text)
			if err != nil {
				return fmt.Sprintf("Error decoding hex: %v", err)
			}
			return string(decoded)

		default:
			return fmt.Sprintf("Error: unknown operation %q — use base64_encode, base64_decode, url_encode, url_decode, hex_encode, or hex_decode", op)
		}
	},
}

var RegexMatch = &ToolDef{
	Name:        "regex_match",
	Description: "Apply a regular expression to text: check for a match, find the first match, find all matches, or replace matches",
	Args: []ToolArg{
		{Name: "text", Description: "Input text to search", Required: true},
		{Name: "pattern", Description: "Regular expression pattern (Go syntax)", Required: true},
		{Name: "operation", Description: "Operation: match, find, find_all, replace (default: find)", Required: false},
		{Name: "replacement", Description: "Replacement string for 'replace' (supports $1, $2 capture groups)", Required: false},
	},
	Execute: func(args map[string]string) string {
		text := args["text"]
		pattern := args["pattern"]
		if text == "" || pattern == "" {
			return "Error: text and pattern are required"
		}
		op := strings.ToLower(strings.TrimSpace(args["operation"]))
		if op == "" {
			op = "find"
		}

		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Sprintf("Error: invalid regex pattern: %v", err)
		}

		switch op {
		case "match":
			if re.MatchString(text) {
				return "true — pattern matched"
			}
			return "false — no match"

		case "find":
			m := re.FindString(text)
			if m == "" {
				return "No match found"
			}
			return fmt.Sprintf("Match: %q", m)

		case "find_all":
			matches := re.FindAllString(text, -1)
			if len(matches) == 0 {
				return "No matches found"
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Found %d match(es):\n", len(matches)))
			for i, m := range matches {
				sb.WriteString(fmt.Sprintf("%d: %q\n", i+1, m))
			}
			return strings.TrimRight(sb.String(), "\n")

		case "replace":
			replacement := args["replacement"]
			return re.ReplaceAllString(text, replacement)

		default:
			return fmt.Sprintf("Error: unknown operation %q — use match, find, find_all, or replace", op)
		}
	},
}

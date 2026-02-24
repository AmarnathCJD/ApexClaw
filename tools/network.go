package tools

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var Weather = &ToolDef{
	Name:        "weather",
	Description: "Get current weather and multi-day forecast for any city or location",
	Args: []ToolArg{
		{Name: "location", Description: "City or location name (e.g. 'Paris', 'New York', 'Mumbai')", Required: true},
		{Name: "days", Description: "Number of forecast days to include (1–7, default 1)", Required: false},
	},
	Execute: func(args map[string]string) string {
		location := strings.TrimSpace(args["location"])
		if location == "" {
			return "Error: location is required"
		}
		days := args["days"]
		if days == "" {
			days = "1"
		}

		geoURL := fmt.Sprintf(
			"https://geocoding-api.open-meteo.com/v1/search?name=%s&count=1&language=en&format=json",
			url.QueryEscape(location),
		)
		client := &http.Client{Timeout: 15 * time.Second}
		req, _ := http.NewRequest("GET", geoURL, nil)
		req.Header.Set("User-Agent", "ApexClaw/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error geocoding location: %v", err)
		}
		defer resp.Body.Close()
		geoBody, _ := io.ReadAll(resp.Body)

		var geoResult struct {
			Results []struct {
				Name      string  `json:"name"`
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
				Country   string  `json:"country"`
			} `json:"results"`
		}
		if err := json.Unmarshal(geoBody, &geoResult); err != nil || len(geoResult.Results) == 0 {
			return fmt.Sprintf("Location not found: %s", location)
		}
		place := geoResult.Results[0]

		weatherURL := fmt.Sprintf(
			"https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f"+
				"&current=temperature_2m,apparent_temperature,relative_humidity_2m,wind_speed_10m,weather_code,precipitation"+
				"&daily=temperature_2m_max,temperature_2m_min,precipitation_sum,weather_code"+
				"&forecast_days=%s&timezone=auto",
			place.Latitude, place.Longitude, days,
		)
		req2, _ := http.NewRequest("GET", weatherURL, nil)
		req2.Header.Set("User-Agent", "ApexClaw/1.0")
		resp2, err := client.Do(req2)
		if err != nil {
			return fmt.Sprintf("Error fetching weather: %v", err)
		}
		defer resp2.Body.Close()
		wBody, _ := io.ReadAll(resp2.Body)

		var w struct {
			Current struct {
				Temperature float64 `json:"temperature_2m"`
				FeelsLike   float64 `json:"apparent_temperature"`
				Humidity    int     `json:"relative_humidity_2m"`
				WindSpeed   float64 `json:"wind_speed_10m"`
				WeatherCode int     `json:"weather_code"`
				Precip      float64 `json:"precipitation"`
			} `json:"current"`
			Daily struct {
				Time        []string  `json:"time"`
				TempMax     []float64 `json:"temperature_2m_max"`
				TempMin     []float64 `json:"temperature_2m_min"`
				PrecipSum   []float64 `json:"precipitation_sum"`
				WeatherCode []int     `json:"weather_code"`
			} `json:"daily"`
		}
		if err := json.Unmarshal(wBody, &w); err != nil {
			return fmt.Sprintf("Error parsing weather data: %v", err)
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Weather — %s, %s\n", place.Name, place.Country))
		sb.WriteString(strings.Repeat("─", 36) + "\n")
		sb.WriteString(fmt.Sprintf("Condition:   %s\n", wmoCondition(w.Current.WeatherCode)))
		sb.WriteString(fmt.Sprintf("Temperature: %.1f°C (feels %.1f°C)\n", w.Current.Temperature, w.Current.FeelsLike))
		sb.WriteString(fmt.Sprintf("Humidity:    %d%%\n", w.Current.Humidity))
		sb.WriteString(fmt.Sprintf("Wind:        %.1f km/h\n", w.Current.WindSpeed))
		if w.Current.Precip > 0 {
			sb.WriteString(fmt.Sprintf("Rain now:    %.1f mm\n", w.Current.Precip))
		}
		if len(w.Daily.Time) > 1 {
			sb.WriteString("\nForecast:\n")
			for i, day := range w.Daily.Time {
				line := fmt.Sprintf("  %-12s %s, %.0f–%.0f°C", day, wmoCondition(w.Daily.WeatherCode[i]), w.Daily.TempMin[i], w.Daily.TempMax[i])
				if len(w.Daily.PrecipSum) > i && w.Daily.PrecipSum[i] > 0 {
					line += fmt.Sprintf(", rain %.1fmm", w.Daily.PrecipSum[i])
				}
				sb.WriteString(line + "\n")
			}
		}
		return strings.TrimRight(sb.String(), "\n")
	},
}

func wmoCondition(code int) string {
	switch {
	case code == 0:
		return "Clear sky"
	case code == 1:
		return "Mainly clear"
	case code == 2:
		return "Partly cloudy"
	case code == 3:
		return "Overcast"
	case code >= 45 && code <= 48:
		return "Fog"
	case code >= 51 && code <= 55:
		return "Drizzle"
	case code >= 61 && code <= 65:
		return "Rain"
	case code >= 66 && code <= 67:
		return "Freezing rain"
	case code >= 71 && code <= 77:
		return "Snow"
	case code >= 80 && code <= 82:
		return "Rain showers"
	case code >= 85 && code <= 86:
		return "Snow showers"
	case code == 95:
		return "Thunderstorm"
	case code >= 96 && code <= 99:
		return "Thunderstorm with hail"
	default:
		return fmt.Sprintf("Code %d", code)
	}
}

var IPLookup = &ToolDef{
	Name:        "ip_lookup",
	Description: "Look up geolocation, ISP, and timezone info for any IP address (leave empty to check your own IP)",
	Args: []ToolArg{
		{Name: "ip", Description: "IP address to look up (IPv4 or IPv6). Leave empty to look up the server's own public IP.", Required: false},
	},
	Execute: func(args map[string]string) string {
		ip := strings.TrimSpace(args["ip"])
		var apiURL string
		if ip == "" {
			apiURL = "http://ip-api.com/json/?fields=status,message,country,countryCode,regionName,city,zip,lat,lon,timezone,isp,org,as,query"
		} else {
			apiURL = fmt.Sprintf("http://ip-api.com/json/%s?fields=status,message,country,countryCode,regionName,city,zip,lat,lon,timezone,isp,org,as,query", url.PathEscape(ip))
		}

		client := &http.Client{Timeout: 10 * time.Second}
		req, _ := http.NewRequest("GET", apiURL, nil)
		req.Header.Set("User-Agent", "ApexClaw/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error fetching IP info: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		var r struct {
			Status      string  `json:"status"`
			Message     string  `json:"message"`
			Query       string  `json:"query"`
			Country     string  `json:"country"`
			CountryCode string  `json:"countryCode"`
			RegionName  string  `json:"regionName"`
			City        string  `json:"city"`
			Zip         string  `json:"zip"`
			Lat         float64 `json:"lat"`
			Lon         float64 `json:"lon"`
			Timezone    string  `json:"timezone"`
			ISP         string  `json:"isp"`
			Org         string  `json:"org"`
			AS          string  `json:"as"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return fmt.Sprintf("Error parsing response: %v", err)
		}
		if r.Status != "success" {
			return fmt.Sprintf("Lookup failed: %s", r.Message)
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("IP Lookup: %s\n", r.Query))
		sb.WriteString(strings.Repeat("─", 30) + "\n")
		sb.WriteString(fmt.Sprintf("Location:  %s, %s, %s (%s)\n", r.City, r.RegionName, r.Country, r.CountryCode))
		if r.Zip != "" {
			sb.WriteString(fmt.Sprintf("ZIP:       %s\n", r.Zip))
		}
		sb.WriteString(fmt.Sprintf("Coords:    %.4f, %.4f\n", r.Lat, r.Lon))
		sb.WriteString(fmt.Sprintf("Timezone:  %s\n", r.Timezone))
		sb.WriteString(fmt.Sprintf("ISP:       %s\n", r.ISP))
		if r.Org != "" {
			sb.WriteString(fmt.Sprintf("Org:       %s\n", r.Org))
		}
		if r.AS != "" {
			sb.WriteString(fmt.Sprintf("AS:        %s\n", r.AS))
		}
		return strings.TrimRight(sb.String(), "\n")
	},
}

var DNSLookup = &ToolDef{
	Name:        "dns_lookup",
	Description: "Query DNS records for a domain (A, MX, TXT, CNAME, NS, or all)",
	Args: []ToolArg{
		{Name: "domain", Description: "Domain name to query (e.g. 'google.com')", Required: true},
		{Name: "type", Description: "Record type: A, MX, TXT, CNAME, NS, or all (default: all)", Required: false},
	},
	Execute: func(args map[string]string) string {
		domain := strings.TrimSpace(args["domain"])
		if domain == "" {
			return "Error: domain is required"
		}
		recType := strings.ToUpper(strings.TrimSpace(args["type"]))
		if recType == "" {
			recType = "ALL"
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("DNS: %s\n", domain))
		sb.WriteString(strings.Repeat("─", 30) + "\n")

		found := false
		if recType == "A" || recType == "ALL" {
			addrs, err := net.LookupHost(domain)
			if err == nil && len(addrs) > 0 {
				sb.WriteString("A / AAAA:\n")
				for _, a := range addrs {
					sb.WriteString(fmt.Sprintf("  %s\n", a))
				}
				found = true
			}
		}
		if recType == "MX" || recType == "ALL" {
			mxs, err := net.LookupMX(domain)
			if err == nil && len(mxs) > 0 {
				sb.WriteString("MX:\n")
				for _, mx := range mxs {
					sb.WriteString(fmt.Sprintf("  pref=%d  %s\n", mx.Pref, mx.Host))
				}
				found = true
			}
		}
		if recType == "TXT" || recType == "ALL" {
			txts, err := net.LookupTXT(domain)
			if err == nil && len(txts) > 0 {
				sb.WriteString("TXT:\n")
				for _, t := range txts {
					sb.WriteString(fmt.Sprintf("  %s\n", t))
				}
				found = true
			}
		}
		if recType == "CNAME" || recType == "ALL" {
			cname, err := net.LookupCNAME(domain)
			if err == nil && cname != domain+"." && cname != domain {
				sb.WriteString(fmt.Sprintf("CNAME: %s\n", cname))
				found = true
			}
		}
		if recType == "NS" || recType == "ALL" {
			nss, err := net.LookupNS(domain)
			if err == nil && len(nss) > 0 {
				sb.WriteString("NS:\n")
				for _, ns := range nss {
					sb.WriteString(fmt.Sprintf("  %s\n", ns.Host))
				}
				found = true
			}
		}
		if !found {
			return fmt.Sprintf("No DNS records found for %s (type: %s)", domain, recType)
		}
		return strings.TrimRight(sb.String(), "\n")
	},
}

var HTTPRequest = &ToolDef{
	Name:        "http_request",
	Description: "Make any HTTP request (GET/POST/PUT/DELETE/PATCH) with custom headers and body",
	Args: []ToolArg{
		{Name: "url", Description: "Full URL to send the request to", Required: true},
		{Name: "method", Description: "HTTP method: GET, POST, PUT, DELETE, PATCH (default: GET)", Required: false},
		{Name: "headers", Description: `JSON object of request headers, e.g. {"Authorization":"Bearer token","Content-Type":"application/json"}`, Required: false},
		{Name: "body", Description: "Request body string (used for POST/PUT/PATCH)", Required: false},
		{Name: "timeout", Description: "Timeout in seconds (default: 15)", Required: false},
	},
	Execute: func(args map[string]string) string {
		rawURL := strings.TrimSpace(args["url"])
		if rawURL == "" {
			return "Error: url is required"
		}
		method := strings.ToUpper(strings.TrimSpace(args["method"]))
		if method == "" {
			method = "GET"
		}
		timeoutSec := 15
		if t := args["timeout"]; t != "" {
			fmt.Sscanf(t, "%d", &timeoutSec)
		}

		var bodyReader io.Reader
		if b := args["body"]; b != "" {
			bodyReader = strings.NewReader(b)
		}

		req, err := http.NewRequest(method, rawURL, bodyReader)
		if err != nil {
			return fmt.Sprintf("Error building request: %v", err)
		}
		req.Header.Set("User-Agent", "ApexClaw/1.0")

		if hdrs := strings.TrimSpace(args["headers"]); hdrs != "" {
			var headerMap map[string]string
			if err := json.Unmarshal([]byte(hdrs), &headerMap); err != nil {
				return fmt.Sprintf("Error parsing headers JSON: %v", err)
			}
			for k, v := range headerMap {
				req.Header.Set(k, v)
			}
		}

		client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Request error: %v", err)
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
		if err != nil {
			return fmt.Sprintf("Error reading response: %v", err)
		}
		text := strings.TrimSpace(string(respBody))
		if len(text) > 4000 {
			text = text[:4000] + "\n...(truncated)"
		}
		return fmt.Sprintf("HTTP %d %s\n\n%s", resp.StatusCode, resp.Status, text)
	},
}

type feedRSSEntry struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	PubDate string `xml:"pubDate"`
	Desc    string `xml:"description"`
}

type feedRSSChannel struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Title string         `xml:"title"`
		Items []feedRSSEntry `xml:"item"`
	} `xml:"channel"`
}

type feedAtomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type feedAtomEntry struct {
	Title   string       `xml:"title"`
	Link    feedAtomLink `xml:"link"`
	Updated string       `xml:"updated"`
	Summary string       `xml:"summary"`
	Content string       `xml:"content"`
}

type feedAtomChannel struct {
	XMLName xml.Name        `xml:"feed"`
	Title   string          `xml:"title"`
	Entries []feedAtomEntry `xml:"entry"`
}

var RSSFeed = &ToolDef{
	Name:        "rss_feed",
	Description: "Fetch and read an RSS or Atom feed, returning the latest items with titles, links, and summaries",
	Args: []ToolArg{
		{Name: "url", Description: "URL of the RSS or Atom feed", Required: true},
		{Name: "limit", Description: "Number of items to return (default: 5, max: 20)", Required: false},
	},
	Execute: func(args map[string]string) string {
		feedURL := strings.TrimSpace(args["url"])
		if feedURL == "" {
			return "Error: url is required"
		}
		limit := 5
		if l := args["limit"]; l != "" {
			fmt.Sscanf(l, "%d", &limit)
		}
		if limit < 1 {
			limit = 1
		}
		if limit > 20 {
			limit = 20
		}

		client := &http.Client{Timeout: 15 * time.Second}
		req, _ := http.NewRequest("GET", feedURL, nil)
		req.Header.Set("User-Agent", "ApexClaw/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error fetching feed: %v", err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))

		xmlStr := strings.ReplaceAll(string(data), ` xmlns="http://www.w3.org/2005/Atom"`, "")
		xmlStr = strings.ReplaceAll(xmlStr, `xmlns="http://www.w3.org/2005/Atom"`, "")

		var sb strings.Builder

		var rss feedRSSChannel
		if err := xml.Unmarshal([]byte(xmlStr), &rss); err == nil && len(rss.Channel.Items) > 0 {
			if rss.Channel.Title != "" {
				sb.WriteString(fmt.Sprintf("Feed: %s\n", rss.Channel.Title))
			}
			sb.WriteString(strings.Repeat("─", 36) + "\n")
			items := rss.Channel.Items
			if len(items) > limit {
				items = items[:limit]
			}
			for i, item := range items {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, item.Title))
				if item.Link != "" {
					sb.WriteString(fmt.Sprintf("   %s\n", item.Link))
				}
				if item.PubDate != "" {
					sb.WriteString(fmt.Sprintf("   %s\n", item.PubDate))
				}
				if item.Desc != "" {
					d := strings.TrimSpace(item.Desc)
					if len(d) > 200 {
						d = d[:200] + "..."
					}
					sb.WriteString(fmt.Sprintf("   %s\n", d))
				}
				sb.WriteString("\n")
			}
			return strings.TrimRight(sb.String(), "\n")
		}

		var atom feedAtomChannel
		if err := xml.Unmarshal([]byte(xmlStr), &atom); err == nil && len(atom.Entries) > 0 {
			if atom.Title != "" {
				sb.WriteString(fmt.Sprintf("Feed: %s\n", atom.Title))
			}
			sb.WriteString(strings.Repeat("─", 36) + "\n")
			entries := atom.Entries
			if len(entries) > limit {
				entries = entries[:limit]
			}
			for i, entry := range entries {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, entry.Title))
				if entry.Link.Href != "" {
					sb.WriteString(fmt.Sprintf("   %s\n", entry.Link.Href))
				}
				if entry.Updated != "" {
					sb.WriteString(fmt.Sprintf("   %s\n", entry.Updated))
				}
				desc := entry.Summary
				if desc == "" {
					desc = entry.Content
				}
				if desc != "" {
					desc = strings.TrimSpace(desc)
					if len(desc) > 200 {
						desc = desc[:200] + "..."
					}
					sb.WriteString(fmt.Sprintf("   %s\n", desc))
				}
				sb.WriteString("\n")
			}
			return strings.TrimRight(sb.String(), "\n")
		}

		return "Error: could not parse feed as RSS or Atom XML"
	},
}

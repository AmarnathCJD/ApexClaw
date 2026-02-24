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

var NavGeocode = &ToolDef{
	Name:        "nav_geocode",
	Description: "Convert a place name or address into latitude and longitude coordinates. Returns the full display name and coordinates.",
	Args: []ToolArg{
		{Name: "query", Description: "Location name or address to search for (e.g., 'Cochin', 'London Eye').", Required: true},
	},
	Execute: func(args map[string]string) string {
		query := args["query"]
		if query == "" {
			return "Error: query is required"
		}

		apiURL := "https://nominatim.openstreetmap.org/search?format=json&limit=1&q=" + url.QueryEscape(query)
		client := &http.Client{Timeout: 10 * time.Second}
		req, _ := http.NewRequest("GET", apiURL, nil)

		req.Header.Set("User-Agent", "ApexClawAIAssistant/1.0")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error fetching geocode: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Sprintf("Error reading geocode response: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Sprintf("Geocoding failed (status %d): %s", resp.StatusCode, string(body))
		}

		var results []struct {
			DisplayName string `json:"display_name"`
			Lat         string `json:"lat"`
			Lon         string `json:"lon"`
		}

		if err := json.Unmarshal(body, &results); err != nil {
			return fmt.Sprintf("Error parsing geocode JSON: %v", err)
		}

		if len(results) == 0 {
			return fmt.Sprintf("No location found for: %s", query)
		}

		return fmt.Sprintf("Location: %s\nLatitude: %s\nLongitude: %s", results[0].DisplayName, results[0].Lat, results[0].Lon)
	},
}

var NavRoute = &ToolDef{
	Name:        "nav_route",
	Description: "Calculate driving distance, duration, and route summary between two coordinates using Project OSRM. Inputs must be 'lon,lat'. Use nav_geocode first if you only have place names.",
	Args: []ToolArg{
		{Name: "start", Description: "Starting coordinates as 'longitude,latitude' (e.g. '76.4019,10.1511')", Required: true},
		{Name: "destination", Description: "Destination coordinates as 'longitude,latitude' (e.g. '75.9509,11.1378')", Required: true},
	},
	Execute: func(args map[string]string) string {
		start := args["start"]
		destination := args["destination"]

		if start == "" || destination == "" {
			return "Error: both start and destination coordinates are required"
		}

		apiURL := fmt.Sprintf("https://router.project-osrm.org/route/v1/driving/%s;%s?overview=false", url.PathEscape(start), url.PathEscape(destination))

		client := &http.Client{Timeout: 15 * time.Second}
		req, _ := http.NewRequest("GET", apiURL, nil)
		req.Header.Set("User-Agent", "ApexClawAIAssistant/1.0")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error fetching route from OSRM: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Sprintf("Error reading route response: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Sprintf("OSRM routing failed (status %d): %s", resp.StatusCode, string(body))
		}

		var result struct {
			Code   string `json:"code"`
			Routes []struct {
				Distance float64 `json:"distance"`
				Duration float64 `json:"duration"`
			} `json:"routes"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Sprintf("Error parsing OSRM JSON: %v\nRaw: %s", err, string(body))
		}

		if result.Code != "Ok" || len(result.Routes) == 0 {
			return fmt.Sprintf("Could not calculate route. Code: %s", result.Code)
		}

		distKm := result.Routes[0].Distance / 1000.0
		durMin := result.Routes[0].Duration / 60.0

		return fmt.Sprintf("Driving Route calculated successfully:\nDistance: %.2f km\nEstimated Duration: %.0f minutes", distKm, durMin)
	},
}

var NavSunshade = &ToolDef{
	Name:        "nav_sunshade",
	Description: "Determine which side of the vehicle (left or right) will be exposed to the sun during a journey, to help choose the best side to sit on (the shady side).",
	Args: []ToolArg{
		{Name: "start", Description: "Starting coordinates as 'latitude,longitude' (e.g. '11.447,75.830')", Required: true},
		{Name: "destination", Description: "Destination coordinates as 'latitude,longitude' (e.g. '11.245,75.775')", Required: true},
		{Name: "date", Description: "Journey date in YYYY-MM-DD format", Required: true},
		{Name: "time", Description: "Journey start time in HH:mm format", Required: true},
		{Name: "tz", Description: "Timezone offset (e.g. 5.5 for IST). Defaults to 5.5 if omitted.", Required: false},
		{Name: "duration", Description: "Expected journey duration in seconds. Defaults to 0 if omitted.", Required: false},
	},
	Execute: func(args map[string]string) string {
		start := args["start"]
		destination := args["destination"]
		date := args["date"]
		tm := args["time"]

		if start == "" || destination == "" || date == "" || tm == "" {
			return "Error: start, destination, date, and time are required"
		}

		tz := args["tz"]
		if tz == "" {
			tz = "5.5"
		}

		duration := args["duration"]
		if duration == "" {
			duration = "0"
		}

		startParts := strings.Split(start, ",")
		if len(startParts) != 2 {
			return "Error: start coordinate must be in 'lat,lon' format"
		}
		lat1 := strings.TrimSpace(startParts[0])
		lon1 := strings.TrimSpace(startParts[1])

		destParts := strings.Split(destination, ",")
		if len(destParts) != 2 {
			return "Error: destination coordinate must be in 'lat,lon' format"
		}
		lat2 := strings.TrimSpace(destParts[0])
		lon2 := strings.TrimSpace(destParts[1])

		apiURL := fmt.Sprintf("https://sun.amithv.workers.dev/?lat1=%s&lon1=%s&lat2=%s&lon2=%s&date=%s&time=%s&tz=%s&duration=%s",
			url.QueryEscape(lat1), url.QueryEscape(lon1),
			url.QueryEscape(lat2), url.QueryEscape(lon2),
			url.QueryEscape(date), url.QueryEscape(tm),
			url.QueryEscape(tz), url.QueryEscape(duration),
		)

		client := &http.Client{Timeout: 15 * time.Second}
		req, _ := http.NewRequest("GET", apiURL, nil)

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error fetching sunshade info: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Sprintf("Error reading sunshade response: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Sprintf("Sunshade calculation failed (status %d): %s", resp.StatusCode, string(body))
		}

		var result struct {
			LeftPercentage  string `json:"leftPercentage"`
			RightPercentage string `json:"rightPercentage"`
			NoSunPercentage string `json:"noSunPercentage"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			bodyStr := string(body)
			if len(bodyStr) > 200 {
				bodyStr = bodyStr[:200] + "..."
			}
			return fmt.Sprintf("Error parsing JSON: %v. Raw: %s", err, bodyStr)
		}

		return fmt.Sprintf("Sun Exposure Analysis:\nLeft Side Sun Exposure: %s%%\nRight Side Sun Exposure: %s%%\nNo Sun / Shaded Exposure: %s%%", result.LeftPercentage, result.RightPercentage, result.NoSunPercentage)
	},
}

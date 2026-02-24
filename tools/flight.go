package tools

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

var FlightAirportSearch = &ToolDef{
	Name:        "flight_airport_search",
	Description: "Search FlightRadar24 for airports to get the airport code (e.g., query 'cok').",
	Args: []ToolArg{
		{Name: "query", Description: "Search query text (airport code, city).", Required: true},
		{Name: "limit", Description: "Number of maximum results to return. Default is 10.", Required: false},
	},
	Execute: func(args map[string]string) string {
		query := args["query"]
		if query == "" {
			return "Error: query is required"
		}

		limit := args["limit"]
		if limit == "" {
			limit = "10"
		}

		params := url.Values{}
		params.Add("query", query)
		params.Add("limit", limit)
		params.Add("type", "airport")

		apiURL := "https://www.flightradar24.com/v1/search/web/find?" + params.Encode()
		return doFlightRadarRequest(apiURL)
	},
}

var FlightRouteSearch = &ToolDef{
	Name:        "flight_route_search",
	Description: "Search FlightRadar24 for flights on a specific route (e.g., query 'COK-CCJ') to get current flights.",
	Args: []ToolArg{
		{Name: "query", Description: "Route query (e.g. 'COK-CCJ').", Required: true},
		{Name: "limit", Description: "Number of maximum results to return. Default is 50.", Required: false},
	},
	Execute: func(args map[string]string) string {
		query := args["query"]
		if query == "" {
			return "Error: query is required"
		}

		limit := args["limit"]
		if limit == "" {
			limit = "50"
		}

		params := url.Values{}
		params.Add("query", query)
		params.Add("limit", limit)

		apiURL := "https://www.flightradar24.com/v1/search/web/find?" + params.Encode()
		return doFlightRadarRequest(apiURL)
	},
}

var FlightCountries = &ToolDef{
	Name:        "flight_countries",
	Description: "Get the list of countries from FlightRadar24 to find country codes/airports.",
	Args:        []ToolArg{},
	Execute: func(args map[string]string) string {
		apiURL := "https://www.flightradar24.com/mobile/countries"
		return doFlightRadarRequest(apiURL)
	},
}

func doFlightRadarRequest(apiURL string) string {
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("Error fetching FlightRadar24 API: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("Error reading response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("Error from FlightRadar24 API (status %d): %s", resp.StatusCode, string(body))
	}

	text := string(body)
	if len(text) > 8000 {
		text = text[:8000] + "\n...(truncated)"
	}
	return text
}

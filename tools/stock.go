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

var StockPrice = &ToolDef{
	Name:        "stock_price",
	Description: "Get live stock/crypto/forex quotes. For stocks use ticker symbols like AAPL, TSLA, RELIANCE.NS, INFY.BO. For crypto use BTC-USD, ETH-USD. For forex use EURUSD=X.",
	Args: []ToolArg{
		{Name: "symbol", Description: "Ticker symbol(s), comma-separated for multiple (e.g. 'AAPL,TSLA' or 'BTC-USD')", Required: true},
	},
	Execute: func(args map[string]string) string {
		rawSymbols := strings.TrimSpace(args["symbol"])
		if rawSymbols == "" {
			return "Error: symbol is required"
		}

		symbols := strings.Split(rawSymbols, ",")
		var results []string

		for _, sym := range symbols {
			sym = strings.TrimSpace(strings.ToUpper(sym))
			if sym == "" {
				continue
			}
			result := fetchYahooQuote(sym)
			results = append(results, result)
		}

		if len(results) == 0 {
			return "No results"
		}
		return strings.Join(results, "\n")
	},
}

func fetchYahooQuote(symbol string) string {
	apiURL := fmt.Sprintf(
		"https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=1d",
		url.PathEscape(symbol),
	)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return fmt.Sprintf("%s: request error: %v", symbol, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("%s: fetch error: %v", symbol, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var data struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Symbol             string  `json:"symbol"`
					RegularMarketPrice float64 `json:"regularMarketPrice"`
					PreviousClose      float64 `json:"previousClose"`
					RegularMarketTime  int64   `json:"regularMarketTime"`
					Currency           string  `json:"currency"`
					ExchangeName       string  `json:"exchangeName"`
					MarketState        string  `json:"marketState"`
					RegularMarketHigh  float64 `json:"regularMarketDayHigh"`
					RegularMarketLow   float64 `json:"regularMarketDayLow"`
					RegularMarketOpen  float64 `json:"regularMarketOpen"`
					RegularMarketVol   int64   `json:"regularMarketVolume"`
				} `json:"meta"`
			} `json:"result"`
			Error *struct {
				Code        string `json:"code"`
				Description string `json:"description"`
			} `json:"error"`
		} `json:"chart"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Sprintf("%s: parse error", symbol)
	}
	if data.Chart.Error != nil {
		return fmt.Sprintf("%s: %s", symbol, data.Chart.Error.Description)
	}
	if len(data.Chart.Result) == 0 {
		return fmt.Sprintf("%s: no data returned", symbol)
	}

	m := data.Chart.Result[0].Meta
	change := m.RegularMarketPrice - m.PreviousClose
	changePct := 0.0
	if m.PreviousClose != 0 {
		changePct = (change / m.PreviousClose) * 100
	}
	sign := "+"
	if change < 0 {
		sign = ""
	}

	t := time.Unix(m.RegularMarketTime, 0)
	return fmt.Sprintf(
		"%s (%s) — %s %.4f %s%.4f (%s%.2f%%)\nO: %.4f  H: %.4f  L: %.4f  Vol: %d  [%s @ %s]",
		m.Symbol, m.ExchangeName, m.Currency, m.RegularMarketPrice,
		sign, change, sign, changePct,
		m.RegularMarketOpen, m.RegularMarketHigh, m.RegularMarketLow, m.RegularMarketVol,
		m.MarketState, t.Format("02 Jan 15:04 MST"),
	)
}

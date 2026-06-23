package model

import "encoding/json"

type UpstreamFile struct {
	Type   string          `json:"type"`
	File   json.RawMessage `json:"file"`
	ID     string          `json:"id"`
	URL    string          `json:"url"`
	Name   string          `json:"name"`
	Status string          `json:"status"`
	Size   int64           `json:"size"`
	Error  string          `json:"error"`
	ItemID string          `json:"itemId"`
	Media  string          `json:"media"`
}

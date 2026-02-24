package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/google/uuid"
)

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

type fileUploadMeta struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Meta     struct {
		Size int64 `json:"size"`
	} `json:"meta"`
}

func UploadImageData(token string, data []byte, filename string) (*UpstreamFile, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return nil, fmt.Errorf("write data: %w", err)
	}
	writer.Close()

	req, err := http.NewRequest("POST", "https://chat.z.ai/api/v1/files/", &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Origin", "https://chat.z.ai")
	req.Header.Set("Referer", "https://chat.z.ai/")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("upload failed status %d: %s", resp.StatusCode, string(body))
	}

	var meta fileUploadMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &UpstreamFile{
		Type:   "image",
		File:   json.RawMessage(body),
		ID:     meta.ID,
		URL:    fmt.Sprintf("/api/v1/files/%s", meta.ID),
		Name:   meta.Filename,
		Status: "uploaded",
		Size:   meta.Meta.Size,
		ItemID: uuid.New().String(),
		Media:  "image",
	}, nil
}

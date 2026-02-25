package tools

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type DocumentType string

const (
	DocTypePDF      DocumentType = "pdf"
	DocTypeTXT      DocumentType = "txt"
	DocTypeMarkdown DocumentType = "md"
	DocTypeImage    DocumentType = "image"
)

func ExtractTextFromFile(filePath string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file")
	}

	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".txt", ".md", ".log", ".json", ".html", ".xml", ".csv":
		return readPlainTextFile(filePath)
	case ".pdf":
		//return extractPDFText(filePath)
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp":
		//return extractImageText(filePath)
	default:
		return "", fmt.Errorf("unsupported file type: %s", ext)
	}

	return "", nil
}

func readPlainTextFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	text := strings.TrimSpace(string(content))
	if len(text) > 50000 {
		text = text[:50000] + "\n[... truncated due to length ...]"
	}
	return text, nil
}

var ReadDocument = &ToolDef{
	Name:        "read_document",
	Description: "Read and extract text from documents (PDF, images, text files, markdown). Returns extracted text content.",
	Args: []ToolArg{
		{Name: "path", Description: "Path to the document file", Required: true},
		{Name: "max_chars", Description: "Maximum characters to return (default 10000)", Required: false},
	},
	Execute: func(args map[string]string) string {
		path := strings.TrimSpace(args["path"])
		if path == "" {
			return "Error: path is required"
		}

		maxChars := 10000
		if mc := strings.TrimSpace(args["max_chars"]); mc != "" {
			var num int
			if _, err := fmt.Sscanf(mc, "%d", &num); err == nil && num > 0 {
				maxChars = num
			}
		}

		text, err := ExtractTextFromFile(path)
		if err != nil {
			return fmt.Sprintf("Error reading document: %v\n\nSupported formats:\n- Text: .txt, .md, .json, .html, .xml, .csv\n- PDF: .pdf (requires system setup)\n- Images: .jpg, .png, .gif (requires Tesseract OCR)\n\nTo setup PDF support:\n  apt-get install poppler-utils\n  pip install pdf2image\n\nTo setup Image OCR:\n  apt-get install tesseract-ocr", err)
		}

		if len(text) > maxChars {
			text = text[:maxChars] + fmt.Sprintf("\n\n[... document truncated, showing first %d characters ...]", maxChars)
		}

		return text
	},
}

var ListDocuments = &ToolDef{
	Name:        "list_documents",
	Description: "List all readable documents in a directory",
	Args: []ToolArg{
		{Name: "path", Description: "Directory path to search", Required: true},
		{Name: "recursive", Description: "Search recursively (true/false)", Required: false},
	},
	Execute: func(args map[string]string) string {
		dirPath := strings.TrimSpace(args["path"])
		if dirPath == "" {
			dirPath = "."
		}

		info, err := os.Stat(dirPath)
		if err != nil {
			return fmt.Sprintf("Error: directory not found: %v", err)
		}
		if !info.IsDir() {
			return "Error: path is not a directory"
		}

		recursive := strings.ToLower(strings.TrimSpace(args["recursive"])) == "true"

		var documents []string
		validExts := map[string]bool{
			".txt": true, ".md": true, ".json": true, ".html": true,
			".xml": true, ".csv": true, ".pdf": true,
			".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
			".bmp": true, ".webp": true,
		}

		filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				if !recursive && path != dirPath {
					return filepath.SkipDir
				}
				return nil
			}

			if validExts[strings.ToLower(filepath.Ext(path))] {
				rel, _ := filepath.Rel(dirPath, path)
				size := info.Size()
				documents = append(documents, fmt.Sprintf("  %s (%d bytes)", rel, size))
			}
			return nil
		})

		if len(documents) == 0 {
			return "No readable documents found"
		}

		return fmt.Sprintf("Found %d documents:\n%s", len(documents), strings.Join(documents, "\n"))
	},
}

var SummarizeDocument = &ToolDef{
	Name:        "summarize_document",
	Description: "Read a document and provide a summary (delegates to AI for analysis)",
	Args: []ToolArg{
		{Name: "path", Description: "Path to the document", Required: true},
		{Name: "style", Description: "Summary style: brief/detailed/bullets", Required: false},
	},
	Execute: func(args map[string]string) string {
		path := strings.TrimSpace(args["path"])
		if path == "" {
			return "Error: path is required"
		}

		text, err := ExtractTextFromFile(path)
		if err != nil {
			return fmt.Sprintf("Error reading document: %v", err)
		}

		style := strings.TrimSpace(args["style"])
		if style == "" {
			style = "brief"
		}

		instruction := fmt.Sprintf("Please provide a %s summary of this document:\n\n%s", style, text)
		return instruction
	},
}

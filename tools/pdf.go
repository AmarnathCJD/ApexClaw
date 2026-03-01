package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// RequiredTools maps tool names to their package names for installation
var RequiredPDFTools = map[string]string{
	"wkhtmltopdf": "wkhtmltopdf",
	"pdftotext":   "poppler-utils",
	"pdfunite":    "poppler-utils",
	"pdfinfo":     "poppler-utils",
	"gs":          "ghostscript",
	"pdflatex":    "texlive-latex-base",
	"xelatex":     "texlive-xetex",
}

// CheckToolInstalled checks if a command-line tool is available
func CheckToolInstalled(toolName string) bool {
	cmd := exec.Command("which", toolName)
	return cmd.Run() == nil
}

// GetMissingTools returns a list of missing PDF tools
func GetMissingTools(requiredTools []string) []string {
	var missing []string
	for _, tool := range requiredTools {
		if !CheckToolInstalled(tool) {
			missing = append(missing, tool)
		}
	}
	return missing
}

// FormatMissingToolsError creates a user-friendly error message about missing tools
func FormatMissingToolsError(missingTools []string) string {
	msg := "⚠ Required system tools not installed:\n\n"
	for _, tool := range missingTools {
		pkg := RequiredPDFTools[tool]
		msg += fmt.Sprintf("  • %s (package: %s)\n", tool, pkg)
	}
	msg += "\nTo install (Linux/Alpine):\n"
	msg += "  apk add wkhtmltopdf poppler-utils ghostscript\n\n"
	msg += "To install (Ubuntu/Debian):\n"
	msg += "  sudo apt-get install wkhtmltopdf poppler-utils ghostscript\n\n"
	msg += "To install (macOS):\n"
	msg += "  brew install wkhtmltopdf poppler ghostscript\n\n"
	msg += "Please install these tools and try again."
	return msg
}

// PDF Creation Tool - creates basic PDF with text content
var PDFCreate = &ToolDef{
	Name:        "pdf_create",
	Description: "Create a new PDF file with text content. Supports title, body text, and basic formatting.",
	Args: []ToolArg{
		{Name: "path", Description: "Output PDF file path", Required: true},
		{Name: "title", Description: "PDF title/heading", Required: false},
		{Name: "content", Description: "PDF body content", Required: true},
	},
	Execute: func(args map[string]string) string {
		path := strings.TrimSpace(args["path"])
		if path == "" {
			return "Error: path is required"
		}

		missing := GetMissingTools([]string{"wkhtmltopdf"})
		if len(missing) > 0 {
			return "⚠ Tool required: wkhtmltopdf\n\nInstall with: apk add wkhtmltopdf (Alpine) or apt-get install wkhtmltopdf (Ubuntu)\n\nContinuing with text fallback..."
		}

		if !strings.HasSuffix(strings.ToLower(path), ".pdf") {
			path = path + ".pdf"
		}

		title := strings.TrimSpace(args["title"])
		content := strings.TrimSpace(args["content"])
		if content == "" {
			return "Error: content is required"
		}

		htmlContent := generateHTMLForPDF(title, content)
		tmpHTML := filepath.Join(os.TempDir(), "pdf_"+randomString(8)+".html")
		defer os.Remove(tmpHTML)

		if err := os.WriteFile(tmpHTML, []byte(htmlContent), 0644); err != nil {
			return fmt.Sprintf("Error creating temporary HTML: %v", err)
		}

		cmd := exec.Command("wkhtmltopdf", "--quiet", tmpHTML, path)
		if err := cmd.Run(); err != nil {
			return convertHTMLtoPDFFallback(tmpHTML, path)
		}

		if _, err := os.Stat(path); err != nil {
			return fmt.Sprintf("Error: PDF file not created at %s", path)
		}

		return fmt.Sprintf("✓ PDF created: %s", path)
	},
}

var PDFExtractText = &ToolDef{
	Name:        "pdf_extract_text",
	Description: "Extract all text content from a PDF file",
	Args: []ToolArg{
		{Name: "path", Description: "PDF file path to extract from", Required: true},
		{Name: "pages", Description: "Page range to extract (e.g., '1-5' or '1,3,5'). Default: all", Required: false},
	},
	Execute: func(args map[string]string) string {
		path := strings.TrimSpace(args["path"])
		if path == "" {
			return "Error: path is required"
		}

		missing := GetMissingTools([]string{"pdftotext"})
		if len(missing) > 0 {
			return "⚠ Tool required: pdftotext (from poppler-utils)\n\nInstall with: apk add poppler-utils (Alpine) or apt-get install poppler-utils (Ubuntu)"
		}

		if _, err := os.Stat(path); err != nil {
			return fmt.Sprintf("Error: PDF file not found: %s", path)
		}

		pageRange := strings.TrimSpace(args["pages"])

		tmpOutput := filepath.Join(os.TempDir(), "pdf_extract_"+randomString(8)+".txt")
		defer os.Remove(tmpOutput)

		cmd := exec.Command("pdftotext")
		if pageRange != "" {
			cmd.Args = append(cmd.Args, "-f", strings.Split(pageRange, "-")[0])
			if parts := strings.Split(pageRange, "-"); len(parts) > 1 {
				cmd.Args = append(cmd.Args, "-l", parts[1])
			}
		}
		cmd.Args = append(cmd.Args, path, tmpOutput)

		if err := cmd.Run(); err != nil {
			return fmt.Sprintf("Error extracting text from PDF: %v", err)
		}

		content, err := os.ReadFile(tmpOutput)
		if err != nil {
			return fmt.Sprintf("Error reading extracted text: %v", err)
		}

		text := string(content)
		if len(text) > 50000 {
			text = text[:50000] + "\n\n[... document truncated, showing first 50000 characters ...]"
		}

		return text
	},
}

var PDFMerge = &ToolDef{
	Name:        "pdf_merge",
	Description: "Merge multiple PDF files into a single PDF",
	Args: []ToolArg{
		{Name: "output", Description: "Output PDF file path", Required: true},
		{Name: "files", Description: "Comma-separated list of PDF file paths to merge", Required: true},
	},
	Execute: func(args map[string]string) string {
		output := strings.TrimSpace(args["output"])
		filesStr := strings.TrimSpace(args["files"])

		if output == "" || filesStr == "" {
			return "Error: output and files are required"
		}

		missing := GetMissingTools([]string{"pdfunite", "gs"})
		if len(missing) > 0 {
			return "⚠ Tools required: pdfunite (poppler-utils) and ghostscript\n\nInstall with: apk add poppler-utils ghostscript (Alpine) or apt-get install poppler-utils ghostscript (Ubuntu)"
		}

		if !strings.HasSuffix(strings.ToLower(output), ".pdf") {
			output = output + ".pdf"
		}

		files := strings.Split(filesStr, ",")
		var cleanFiles []string
		for _, f := range files {
			f = strings.TrimSpace(f)
			if _, err := os.Stat(f); err != nil {
				return fmt.Sprintf("Error: file not found: %s", f)
			}
			cleanFiles = append(cleanFiles, f)
		}

		cmd := exec.Command("pdfunite")
		cmd.Args = append(cmd.Args, cleanFiles...)
		cmd.Args = append(cmd.Args, output)

		if err := cmd.Run(); err != nil {
			return mergePDFWithGhostscript(cleanFiles, output)
		}

		if _, err := os.Stat(output); err != nil {
			return fmt.Sprintf("Error: merged PDF not created at %s", output)
		}

		return fmt.Sprintf("✓ Merged %d PDFs into: %s", len(cleanFiles), output)
	},
}

var PDFSplit = &ToolDef{
	Name:        "pdf_split",
	Description: "Extract a range of pages from PDF and save as new file",
	Args: []ToolArg{
		{Name: "input", Description: "Input PDF file path", Required: true},
		{Name: "output", Description: "Output PDF file path", Required: true},
		{Name: "start_page", Description: "Starting page number (1-indexed)", Required: true},
		{Name: "end_page", Description: "Ending page number (inclusive). Default: same as start_page", Required: false},
	},
	Execute: func(args map[string]string) string {
		input := strings.TrimSpace(args["input"])
		output := strings.TrimSpace(args["output"])
		startStr := strings.TrimSpace(args["start_page"])
		endStr := strings.TrimSpace(args["end_page"])

		if input == "" || output == "" || startStr == "" {
			return "Error: input, output, and start_page are required"
		}

		missing := GetMissingTools([]string{"gs"})
		if len(missing) > 0 {
			return "⚠ Tool required: ghostscript (gs)\n\nInstall with: apk add ghostscript (Alpine) or apt-get install ghostscript (Ubuntu)"
		}

		if _, err := os.Stat(input); err != nil {
			return fmt.Sprintf("Error: input PDF not found: %s", input)
		}

		if !strings.HasSuffix(strings.ToLower(output), ".pdf") {
			output = output + ".pdf"
		}

		startPage, err := strconv.Atoi(startStr)
		if err != nil || startPage < 1 {
			return "Error: start_page must be a positive integer"
		}

		endPage := startPage
		if endStr != "" {
			ep, err := strconv.Atoi(endStr)
			if err == nil && ep >= startPage {
				endPage = ep
			}
		}

		cmd := exec.Command("gs", "-q", "-dNOPAUSE", "-dBATCH", "-dSAFER",
			fmt.Sprintf("-dFirstPage=%d", startPage),
			fmt.Sprintf("-dLastPage=%d", endPage),
			"-sDEVICE=pdfwrite",
			fmt.Sprintf("-sOutputFile=%s", output),
			input)

		if err := cmd.Run(); err != nil {
			return fmt.Sprintf("Error splitting PDF: %v", err)
		}

		if _, err := os.Stat(output); err != nil {
			return fmt.Sprintf("Error: split PDF not created at %s", output)
		}

		return fmt.Sprintf("✓ Extracted pages %d-%d from PDF: %s", startPage, endPage, output)
	},
}

var PDFRotate = &ToolDef{
	Name:        "pdf_rotate",
	Description: "Rotate pages in a PDF file (90, 180, or 270 degrees)",
	Args: []ToolArg{
		{Name: "input", Description: "Input PDF file path", Required: true},
		{Name: "output", Description: "Output PDF file path", Required: true},
		{Name: "degrees", Description: "Rotation angle: 90, 180, or 270", Required: true},
		{Name: "pages", Description: "Page range to rotate (e.g., '1-5'). Default: all", Required: false},
	},
	Execute: func(args map[string]string) string {
		input := strings.TrimSpace(args["input"])
		output := strings.TrimSpace(args["output"])
		degreesStr := strings.TrimSpace(args["degrees"])

		if input == "" || output == "" || degreesStr == "" {
			return "Error: input, output, and degrees are required"
		}

		missing := GetMissingTools([]string{"gs"})
		if len(missing) > 0 {
			return "⚠ Tool required: ghostscript (gs)\n\nInstall with: apk add ghostscript (Alpine) or apt-get install ghostscript (Ubuntu)"
		}

		if _, err := os.Stat(input); err != nil {
			return fmt.Sprintf("Error: input PDF not found: %s", input)
		}

		if !strings.HasSuffix(strings.ToLower(output), ".pdf") {
			output = output + ".pdf"
		}

		degrees, err := strconv.Atoi(degreesStr)
		if err != nil || (degrees != 90 && degrees != 180 && degrees != 270) {
			return "Error: degrees must be 90, 180, or 270"
		}

		cmd := exec.Command("gs", "-q", "-dNOPAUSE", "-dBATCH", "-dSAFER",
			"-sDEVICE=pdfwrite",
			fmt.Sprintf("-sOutputFile=%s", output),
			fmt.Sprintf("-c \"[/Page <</Rotate %d>> /PUT pdfmark\"", degrees),
			input)

		if err := cmd.Run(); err != nil {
			return fmt.Sprintf("Error rotating PDF: %v", err)
		}

		if _, err := os.Stat(output); err != nil {
			return fmt.Sprintf("Error: rotated PDF not created at %s", output)
		}

		return fmt.Sprintf("✓ Rotated PDF pages by %d degrees: %s", degrees, output)
	},
}

var PDFInfo = &ToolDef{
	Name:        "pdf_info",
	Description: "Extract metadata and page count from a PDF file",
	Args: []ToolArg{
		{Name: "path", Description: "PDF file path", Required: true},
	},
	Execute: func(args map[string]string) string {
		path := strings.TrimSpace(args["path"])
		if path == "" {
			return "Error: path is required"
		}

		missing := GetMissingTools([]string{"pdfinfo"})
		if len(missing) > 0 {
			return "⚠ Tool required: pdfinfo (from poppler-utils)\n\nInstall with: apk add poppler-utils (Alpine) or apt-get install poppler-utils (Ubuntu)"
		}

		if _, err := os.Stat(path); err != nil {
			return fmt.Sprintf("Error: PDF file not found: %s", path)
		}

		cmd := exec.Command("pdfinfo", path)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Sprintf("Error reading PDF info: %v", err)
		}

		return fmt.Sprintf("PDF Information:\n%s", string(output))
	},
}

// LaTeX Create - create PDF from LaTeX code
var LaTeXCreate = &ToolDef{
	Name:        "latex_create",
	Description: "Create a PDF from LaTeX source code. Supports pdflatex or xelatex compiler.",
	Args: []ToolArg{
		{Name: "output", Description: "Output PDF file path", Required: true},
		{Name: "latex_code", Description: "LaTeX source code (full document with \\documentclass)", Required: true},
		{Name: "compiler", Description: "LaTeX compiler: pdflatex (default) or xelatex", Required: false},
	},
	Execute: func(args map[string]string) string {
		output := strings.TrimSpace(args["output"])
		latexCode := strings.TrimSpace(args["latex_code"])
		compiler := strings.TrimSpace(args["compiler"])

		if output == "" || latexCode == "" {
			return "Error: output and latex_code are required"
		}

		if compiler == "" {
			compiler = "pdflatex"
		}

		missing := GetMissingTools([]string{compiler})
		if len(missing) > 0 {
			return fmt.Sprintf("⚠ Tool required: %s (from texlive)\n\nInstall with: apk add texlive-latex-base texlive-xetex (Alpine) or apt-get install texlive-latex-base texlive-xetex (Ubuntu)", compiler)
		}

		if !strings.HasSuffix(strings.ToLower(output), ".pdf") {
			output = output + ".pdf"
		}

		// Create temporary directory for LaTeX compilation
		tmpDir := filepath.Join(os.TempDir(), "latex_"+randomString(8))
		if err := os.MkdirAll(tmpDir, 0755); err != nil {
			return fmt.Sprintf("Error creating temp directory: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		// Write LaTeX source to temp file
		tmpTex := filepath.Join(tmpDir, "document.tex")
		if err := os.WriteFile(tmpTex, []byte(latexCode), 0644); err != nil {
			return fmt.Sprintf("Error writing LaTeX source: %v", err)
		}

		// Compile LaTeX to PDF
		cmd := exec.Command(compiler, "-interaction=nonstopmode", "-output-directory="+tmpDir, tmpTex)
		if output, err := cmd.CombinedOutput(); err != nil {
			// Log compilation errors
			errMsg := string(output)
			if strings.Contains(errMsg, "Error") || strings.Contains(errMsg, "error") {
				return fmt.Sprintf("LaTeX compilation error:\n%s", errMsg)
			}
		}

		// Copy generated PDF to output location
		tmpPdf := filepath.Join(tmpDir, "document.pdf")
		if _, err := os.Stat(tmpPdf); err != nil {
			return fmt.Sprintf("Error: PDF not generated. Check LaTeX syntax.\n\nCompiler: %s\nOutput file: %s", compiler, tmpPdf)
		}

		pdfData, err := os.ReadFile(tmpPdf)
		if err != nil {
			return fmt.Sprintf("Error reading generated PDF: %v", err)
		}

		if err := os.WriteFile(output, pdfData, 0644); err != nil {
			return fmt.Sprintf("Error writing output PDF: %v", err)
		}

		return fmt.Sprintf("✓ LaTeX PDF created: %s (compiled with %s)", output, compiler)
	},
}

// LaTeX Edit - edit and save LaTeX source
var LaTeXEdit = &ToolDef{
	Name:        "latex_edit",
	Description: "Create or edit a LaTeX source file (.tex). Does not compile to PDF, just saves the source.",
	Args: []ToolArg{
		{Name: "path", Description: "LaTeX file path (.tex extension)", Required: true},
		{Name: "latex_code", Description: "LaTeX source code to save", Required: true},
		{Name: "mode", Description: "Edit mode: create (overwrite), append, or replace (requires search_text)", Required: false},
		{Name: "search_text", Description: "For replace mode: text to find and replace", Required: false},
		{Name: "replace_text", Description: "For replace mode: replacement text", Required: false},
	},
	Execute: func(args map[string]string) string {
		path := strings.TrimSpace(args["path"])
		latexCode := strings.TrimSpace(args["latex_code"])
		mode := strings.ToLower(strings.TrimSpace(args["mode"]))

		if path == "" || latexCode == "" {
			return "Error: path and latex_code are required"
		}

		if !strings.HasSuffix(strings.ToLower(path), ".tex") {
			path = path + ".tex"
		}

		// Create parent directories if needed
		dir := filepath.Dir(path)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Sprintf("Error creating directory: %v", err)
			}
		}

		var output string

		switch mode {
		case "", "create":
			output = latexCode
		case "append":
			existing, err := os.ReadFile(path)
			if err == nil {
				output = string(existing) + "\n" + latexCode
			} else {
				output = latexCode
			}
		case "replace":
			// Read existing and replace
			existing, err := os.ReadFile(path)
			if err != nil {
				return fmt.Sprintf("Error reading existing file: %v", err)
			}
			searchText := strings.TrimSpace(args["search_text"])
			replaceText := strings.TrimSpace(args["replace_text"])
			if searchText == "" {
				return "Error: search_text required for replace mode"
			}
			output = strings.ReplaceAll(string(existing), searchText, replaceText)
		default:
			return fmt.Sprintf("Error: unsupported mode '%s'. Use: create, append, or replace", mode)
		}

		if err := os.WriteFile(path, []byte(output), 0644); err != nil {
			return fmt.Sprintf("Error writing LaTeX file: %v", err)
		}

		return fmt.Sprintf("✓ LaTeX source saved: %s", path)
	},
}

// LaTeX Compile - compile existing LaTeX file to PDF
var LaTeXCompile = &ToolDef{
	Name:        "latex_compile",
	Description: "Compile an existing LaTeX file (.tex) to PDF.",
	Args: []ToolArg{
		{Name: "input", Description: "Input LaTeX file path (.tex)", Required: true},
		{Name: "output", Description: "Output PDF file path. Default: same as input with .pdf extension", Required: false},
		{Name: "compiler", Description: "LaTeX compiler: pdflatex (default) or xelatex", Required: false},
	},
	Execute: func(args map[string]string) string {
		input := strings.TrimSpace(args["input"])
		output := strings.TrimSpace(args["output"])
		compiler := strings.TrimSpace(args["compiler"])

		if input == "" {
			return "Error: input is required"
		}

		if compiler == "" {
			compiler = "pdflatex"
		}

		missing := GetMissingTools([]string{compiler})
		if len(missing) > 0 {
			return fmt.Sprintf("⚠ Tool required: %s (from texlive)\n\nInstall with: apk add texlive-latex-base texlive-xetex (Alpine) or apt-get install texlive-latex-base texlive-xetex (Ubuntu)", compiler)
		}

		if _, err := os.Stat(input); err != nil {
			return fmt.Sprintf("Error: LaTeX file not found: %s", input)
		}

		if output == "" {
			output = strings.TrimSuffix(input, filepath.Ext(input)) + ".pdf"
		}

		if !strings.HasSuffix(strings.ToLower(output), ".pdf") {
			output = output + ".pdf"
		}

		// Get directory of input file for compilation
		inputDir := filepath.Dir(input)
		inputName := filepath.Base(input)

		// Create temp dir for compilation to keep workspace clean
		tmpDir := filepath.Join(os.TempDir(), "latex_compile_"+randomString(8))
		if err := os.MkdirAll(tmpDir, 0755); err != nil {
			return fmt.Sprintf("Error creating temp directory: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		// Copy input file to temp dir
		inputData, err := os.ReadFile(input)
		if err != nil {
			return fmt.Sprintf("Error reading input file: %v", err)
		}
		tmpInput := filepath.Join(tmpDir, inputName)
		if err := os.WriteFile(tmpInput, inputData, 0644); err != nil {
			return fmt.Sprintf("Error preparing compilation: %v", err)
		}

		// Compile
		cmd := exec.Command(compiler, "-interaction=nonstopmode", "-output-directory="+tmpDir, tmpInput)
		cmd.Dir = inputDir
		if output, err := cmd.CombinedOutput(); err != nil {
			errMsg := string(output)
			if strings.Contains(errMsg, "Error") || strings.Contains(errMsg, "error") {
				return fmt.Sprintf("LaTeX compilation error:\n%s", errMsg)
			}
		}

		// Find generated PDF
		tmpPdf := filepath.Join(tmpDir, strings.TrimSuffix(inputName, filepath.Ext(inputName))+".pdf")
		if _, err := os.Stat(tmpPdf); err != nil {
			return fmt.Sprintf("Error: PDF not generated from %s", input)
		}

		// Copy to output location
		pdfData, err := os.ReadFile(tmpPdf)
		if err != nil {
			return fmt.Sprintf("Error reading generated PDF: %v", err)
		}

		if err := os.WriteFile(output, pdfData, 0644); err != nil {
			return fmt.Sprintf("Error writing output PDF: %v", err)
		}

		return fmt.Sprintf("✓ LaTeX compiled to PDF: %s (using %s)", output, compiler)
	},
}

// Document Search - search within any document/text file
var DocumentSearch = &ToolDef{
	Name:        "document_search",
	Description: "Search for text within a document or file. Returns matching lines with context.",
	Args: []ToolArg{
		{Name: "path", Description: "Document file path", Required: true},
		{Name: "search", Description: "Text to search for (case-insensitive by default)", Required: true},
		{Name: "case_sensitive", Description: "Case-sensitive search (true/false). Default: false", Required: false},
		{Name: "context_lines", Description: "Number of context lines before/after match. Default: 1", Required: false},
	},
	Execute: func(args map[string]string) string {
		path := strings.TrimSpace(args["path"])
		search := strings.TrimSpace(args["search"])

		if path == "" || search == "" {
			return "Error: path and search are required"
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Sprintf("Error reading document: %v", err)
		}

		content := string(data)
		caseSensitive := strings.ToLower(strings.TrimSpace(args["case_sensitive"])) == "true"
		contextLines := 1
		if cl := strings.TrimSpace(args["context_lines"]); cl != "" {
			if n, err := fmt.Sscanf(cl, "%d", &contextLines); err == nil && n == 1 {
				contextLines = n
			}
		}

		lines := strings.Split(content, "\n")
		searchTerm := search
		if !caseSensitive {
			searchTerm = strings.ToLower(search)
		}

		var results []string
		for i, line := range lines {
			lineToCheck := line
			if !caseSensitive {
				lineToCheck = strings.ToLower(line)
			}

			if strings.Contains(lineToCheck, searchTerm) {
				// Build context
				start := i - contextLines
				if start < 0 {
					start = 0
				}
				end := i + contextLines + 1
				if end > len(lines) {
					end = len(lines)
				}

				contextBlock := fmt.Sprintf("\nLine %d:\n", i+1)
				for j := start; j < end; j++ {
					if j == i {
						contextBlock += fmt.Sprintf(">>> %s\n", lines[j])
					} else {
						contextBlock += fmt.Sprintf("    %s\n", lines[j])
					}
				}
				results = append(results, contextBlock)
			}
		}

		if len(results) == 0 {
			return fmt.Sprintf("No matches found for '%s' in %s", search, path)
		}

		return fmt.Sprintf("Found %d match(es):\n%s", len(results), strings.Join(results, ""))
	},
}

func generateHTMLForPDF(title, content string) string {
	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; line-height: 1.6; }
        h1 { color: #333; border-bottom: 2px solid #007bff; padding-bottom: 10px; }
        p { color: #555; white-space: pre-wrap; }
    </style>
</head>
<body>
`
	if title != "" {
		html += fmt.Sprintf("    <h1>%s</h1>\n", title)
	}
	html += fmt.Sprintf("    <p>%s</p>\n", content)
	html += `</body>
</html>`
	return html
}

func convertHTMLtoPDFFallback(htmlPath, pdfPath string) string {
	content, err := os.ReadFile(htmlPath)
	if err != nil {
		return fmt.Sprintf("Error: could not convert to PDF - %v", err)
	}

	text := string(content)
	if err := os.WriteFile(pdfPath, []byte(text), 0644); err != nil {
		return fmt.Sprintf("Error: could not write PDF - %v", err)
	}

	return fmt.Sprintf("⚠ PDF created with fallback method (text format): %s\nInstall wkhtmltopdf for better PDF support", pdfPath)
}

func mergePDFWithGhostscript(files []string, output string) string {
	args := []string{"-q", "-dNOPAUSE", "-dBATCH", "-dSAFER", "-sDEVICE=pdfwrite", fmt.Sprintf("-sOutputFile=%s", output)}
	args = append(args, files...)

	cmd := exec.Command("gs", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("Error: ghostscript merge failed. Install ghostscript: %v", err)
	}

	if _, err := os.Stat(output); err != nil {
		return fmt.Sprintf("Error: merged PDF not created at %s", output)
	}

	return fmt.Sprintf("✓ Merged %d PDFs into: %s", len(files), output)
}

func randomString(length int) string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	var result strings.Builder
	for i := range length {
		result.WriteString(string(chars[i%len(chars)]))
	}
	return result.String()
}

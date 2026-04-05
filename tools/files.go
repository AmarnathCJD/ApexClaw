package tools

import (
	"bufio"
	"fmt"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func sanitizeFileContent(content string) string {
	content = html.UnescapeString(content)
	content = strings.ReplaceAll(content, "</think>", "")
	content = strings.ReplaceAll(content, "<think>", "")
	if !utf8.ValidString(content) {
		content = strings.ToValidUTF8(content, "\uFFFD")
	}
	var b strings.Builder
	b.Grow(len(content))
	for _, r := range content {
		if r == 0 || (r < 0x20 && r != '\t' && r != '\n' && r != '\r') {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// readLines returns all lines of a file (1-indexed externally, 0-indexed slice).
func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}

func writeLines(path string, lines []string) error {
	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// ─── read_file ────────────────────────────────────────────────────────────────

var ReadFile = &ToolDef{
	Name: "read_file",
	Description: "Read a file from disk. Supports optional line range with start_line/end_line (1-based, inclusive). " +
		"Returns content with line numbers prefixed. Handles large files gracefully.",
	Secure: true,
	Args: []ToolArg{
		{Name: "path", Description: "File path to read", Required: true},
		{Name: "start_line", Description: "First line to return (1-based, default: 1)", Required: false},
		{Name: "end_line", Description: "Last line to return (1-based, default: all)", Required: false},
	},
	Execute: func(args map[string]string) string {
		path := args["path"]
		if path == "" {
			return "Error: path is required"
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		if info.IsDir() {
			return fmt.Sprintf("Error: %s is a directory, use list_dir instead", path)
		}

		lines, err := readLines(path)
		if err != nil {
			return fmt.Sprintf("Error reading file: %v", err)
		}
		total := len(lines)

		start := 1
		end := total
		if s := strings.TrimSpace(args["start_line"]); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n >= 1 {
				start = n
			}
		}
		if e := strings.TrimSpace(args["end_line"]); e != "" {
			if n, err := strconv.Atoi(e); err == nil && n >= 1 {
				end = n
			}
		}
		if start > total {
			return fmt.Sprintf("Error: start_line %d exceeds file length %d", start, total)
		}
		if end > total {
			end = total
		}
		if start > end {
			return fmt.Sprintf("Error: start_line %d > end_line %d", start, end)
		}

		const maxLines = 400
		slice := lines[start-1 : end]
		truncated := false
		if len(slice) > maxLines {
			slice = slice[:maxLines]
			truncated = true
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "File: %s (%d lines total)\n", path, total)
		if start > 1 || end < total {
			fmt.Fprintf(&sb, "Showing lines %d–%d\n", start, start+len(slice)-1)
		}
		sb.WriteString("─────\n")
		for i, line := range slice {
			fmt.Fprintf(&sb, "%4d\t%s\n", start+i, line)
		}
		if truncated {
			fmt.Fprintf(&sb, "─────\n...truncated (showing %d/%d lines). Use start_line/end_line to read more.", maxLines, end-start+1)
		}
		return strings.TrimRight(sb.String(), "\n")
	},
}

// ─── write_file ───────────────────────────────────────────────────────────────

var WriteFile = &ToolDef{
	Name: "write_file",
	Description: "Write or overwrite a file. Creates parent directories automatically. " +
		"Backs up existing files to <path>.bak before overwriting. " +
		"Set backup=false to skip backup. Returns byte count and line count on success.",
	Secure: true,
	Args: []ToolArg{
		{Name: "path", Description: "File path to write to", Required: true},
		{Name: "content", Description: "Content to write", Required: true},
		{Name: "backup", Description: "Create .bak backup of existing file (default: true)", Required: false},
	},
	Execute: func(args map[string]string) string {
		path := args["path"]
		if path == "" {
			return "Error: path is required"
		}
		content := sanitizeFileContent(args["content"])

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Sprintf("Error creating directories: %v", err)
		}

		// Backup existing file
		doBackup := args["backup"] != "false"
		if doBackup {
			if _, err := os.Stat(path); err == nil {
				if err := copyFile(path, path+".bak"); err != nil {
					return fmt.Sprintf("Error creating backup: %v", err)
				}
			}
		}

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Sprintf("Error writing file: %v", err)
		}
		lines := strings.Count(content, "\n")
		return fmt.Sprintf("OK — wrote %d bytes, %d lines to %s", len(content), lines, path)
	},
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// ─── edit_file ────────────────────────────────────────────────────────────────

var EditFile = &ToolDef{
	Name: "edit_file",
	Description: "Make targeted edits to a file without rewriting the whole thing. " +
		"Modes:\n" +
		"  replace_lines  — replace lines start_line..end_line with new content\n" +
		"  insert_after   — insert new_content after line_number\n" +
		"  insert_before  — insert new_content before line_number\n" +
		"  delete_lines   — delete lines start_line..end_line\n" +
		"  replace_text   — find exact old_text and replace with new_text (first occurrence)\n" +
		"  replace_all    — find exact old_text and replace all occurrences with new_text\n" +
		"Always backs up to <path>.bak before editing.",
	Secure: true,
	Args: []ToolArg{
		{Name: "path", Description: "File path to edit", Required: true},
		{Name: "mode", Description: "Edit mode: replace_lines | insert_after | insert_before | delete_lines | replace_text | replace_all", Required: true},
		{Name: "old_text", Description: "Text to find (for replace_text / replace_all modes)", Required: false},
		{Name: "new_text", Description: "Replacement text (for replace_text / replace_all modes)", Required: false},
		{Name: "new_content", Description: "New content to insert/replace (for line-based modes)", Required: false},
		{Name: "start_line", Description: "First line number (1-based) for line-based modes", Required: false},
		{Name: "end_line", Description: "Last line number (1-based) for replace_lines / delete_lines", Required: false},
		{Name: "line_number", Description: "Line number for insert_after / insert_before", Required: false},
	},
	Execute: func(args map[string]string) string {
		path := args["path"]
		if path == "" {
			return "Error: path is required"
		}
		mode := strings.TrimSpace(args["mode"])
		if mode == "" {
			return "Error: mode is required"
		}

		// Backup before any edit
		if _, err := os.Stat(path); err == nil {
			if err := copyFile(path, path+".bak"); err != nil {
				return fmt.Sprintf("Error creating backup: %v", err)
			}
		}

		switch mode {
		case "replace_text", "replace_all":
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Sprintf("Error reading file: %v", err)
			}
			old := args["old_text"]
			newT := args["new_text"]
			if old == "" {
				return "Error: old_text is required for replace_text/replace_all"
			}
			content := string(data)
			if !strings.Contains(content, old) {
				// show context around first 200 chars of old_text to help debugging
				preview := old
				if len(preview) > 80 {
					preview = preview[:80] + "..."
				}
				return fmt.Sprintf("Error: old_text not found in file.\nSearched for: %q", preview)
			}
			var newContent string
			if mode == "replace_all" {
				newContent = strings.ReplaceAll(content, old, newT)
			} else {
				newContent = strings.Replace(content, old, newT, 1)
			}
			if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
				return fmt.Sprintf("Error writing file: %v", err)
			}
			count := strings.Count(content, old)
			replaced := 1
			if mode == "replace_all" {
				replaced = count
			}
			return fmt.Sprintf("OK — replaced %d occurrence(s) in %s", replaced, path)

		case "replace_lines", "delete_lines", "insert_after", "insert_before":
			lines, err := readLines(path)
			if err != nil {
				return fmt.Sprintf("Error reading file: %v", err)
			}
			total := len(lines)

			parseInt := func(key string) (int, string) {
				s := strings.TrimSpace(args[key])
				if s == "" {
					return 0, fmt.Sprintf("Error: %s is required for %s mode", key, mode)
				}
				n, err := strconv.Atoi(s)
				if err != nil || n < 1 {
					return 0, fmt.Sprintf("Error: %s must be a positive integer, got %q", key, s)
				}
				return n, ""
			}

			switch mode {
			case "replace_lines":
				start, errs := parseInt("start_line")
				if errs != "" {
					return errs
				}
				end, errs := parseInt("end_line")
				if errs != "" {
					return errs
				}
				if start > total || end > total {
					return fmt.Sprintf("Error: line range %d–%d out of bounds (file has %d lines)", start, end, total)
				}
				if start > end {
					return fmt.Sprintf("Error: start_line %d > end_line %d", start, end)
				}
				newLines := strings.Split(sanitizeFileContent(args["new_content"]), "\n")
				result := append(lines[:start-1], append(newLines, lines[end:]...)...)
				if err := writeLines(path, result); err != nil {
					return fmt.Sprintf("Error writing file: %v", err)
				}
				return fmt.Sprintf("OK — replaced lines %d–%d (%d lines → %d lines) in %s", start, end, end-start+1, len(newLines), path)

			case "delete_lines":
				start, errs := parseInt("start_line")
				if errs != "" {
					return errs
				}
				end, errs := parseInt("end_line")
				if errs != "" {
					return errs
				}
				if start > total || end > total {
					return fmt.Sprintf("Error: line range %d–%d out of bounds (file has %d lines)", start, end, total)
				}
				result := append(lines[:start-1], lines[end:]...)
				if err := writeLines(path, result); err != nil {
					return fmt.Sprintf("Error writing file: %v", err)
				}
				return fmt.Sprintf("OK — deleted lines %d–%d (%d lines removed) from %s", start, end, end-start+1, path)

			case "insert_after", "insert_before":
				lineNum, errs := parseInt("line_number")
				if errs != "" {
					return errs
				}
				if lineNum > total {
					return fmt.Sprintf("Error: line_number %d out of bounds (file has %d lines)", lineNum, total)
				}
				newLines := strings.Split(sanitizeFileContent(args["new_content"]), "\n")
				var result []string
				if mode == "insert_before" {
					result = append(lines[:lineNum-1], append(newLines, lines[lineNum-1:]...)...)
				} else {
					result = append(lines[:lineNum], append(newLines, lines[lineNum:]...)...)
				}
				if err := writeLines(path, result); err != nil {
					return fmt.Sprintf("Error writing file: %v", err)
				}
				pos := "after"
				if mode == "insert_before" {
					pos = "before"
				}
				return fmt.Sprintf("OK — inserted %d lines %s line %d in %s", len(newLines), pos, lineNum, path)
			}
		}
		return fmt.Sprintf("Error: unknown mode %q. Use: replace_lines | insert_after | insert_before | delete_lines | replace_text | replace_all", mode)
	},
}

// ─── grep_file ────────────────────────────────────────────────────────────────

var GrepFile = &ToolDef{
	Name: "grep_file",
	Description: "Search for a pattern in a file or directory. " +
		"Returns matching lines with line numbers and optional context lines. " +
		"Supports regex. Set dir=true to search recursively in a directory.",
	Secure: true,
	Args: []ToolArg{
		{Name: "path", Description: "File path or directory to search", Required: true},
		{Name: "pattern", Description: "Search pattern (regex supported)", Required: true},
		{Name: "context_lines", Description: "Lines of context before and after each match (default: 0)", Required: false},
		{Name: "ignore_case", Description: "Case-insensitive search (true/false, default: false)", Required: false},
		{Name: "max_matches", Description: "Maximum matches to return (default: 50)", Required: false},
	},
	Execute: func(args map[string]string) string {
		path := args["path"]
		pattern := args["pattern"]
		if path == "" || pattern == "" {
			return "Error: path and pattern are required"
		}

		if args["ignore_case"] == "true" {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Sprintf("Error: invalid regex pattern: %v", err)
		}

		ctxLines := 0
		if c := strings.TrimSpace(args["context_lines"]); c != "" {
			if n, err := strconv.Atoi(c); err == nil && n >= 0 {
				ctxLines = n
			}
		}
		maxMatches := 50
		if m := strings.TrimSpace(args["max_matches"]); m != "" {
			if n, err := strconv.Atoi(m); err == nil && n > 0 {
				maxMatches = n
			}
		}

		type match struct {
			file    string
			lineNum int
			lines   []string // context window
			start   int      // offset into lines where the match is
		}

		var matches []match
		totalFound := 0

		searchFile := func(fpath string) {
			lines, err := readLines(fpath)
			if err != nil {
				return
			}
			for i, line := range lines {
				if !re.MatchString(line) {
					continue
				}
				totalFound++
				if len(matches) >= maxMatches {
					continue
				}
				lo := i - ctxLines
				hi := i + ctxLines + 1
				if lo < 0 {
					lo = 0
				}
				if hi > len(lines) {
					hi = len(lines)
				}
				matches = append(matches, match{
					file:    fpath,
					lineNum: i + 1,
					lines:   lines[lo:hi],
					start:   i - lo,
				})
			}
		}

		info, err := os.Stat(path)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		if info.IsDir() {
			filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				// skip binaries heuristically
				ext := strings.ToLower(filepath.Ext(p))
				skip := map[string]bool{".png": true, ".jpg": true, ".gif": true, ".pdf": true, ".zip": true, ".exe": true, ".bin": true, ".so": true}
				if skip[ext] {
					return nil
				}
				searchFile(p)
				return nil
			})
		} else {
			searchFile(path)
		}

		if len(matches) == 0 {
			return fmt.Sprintf("No matches for %q in %s", args["pattern"], path)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Found %d match(es) for %q", totalFound, args["pattern"])
		if totalFound > maxMatches {
			fmt.Fprintf(&sb, " (showing first %d)", maxMatches)
		}
		sb.WriteString("\n")

		lastFile := ""
		for _, m := range matches {
			if m.file != lastFile {
				fmt.Fprintf(&sb, "\n── %s ──\n", m.file)
				lastFile = m.file
			}
			for j, line := range m.lines {
				absLine := m.lineNum - m.start + j
				marker := " "
				if j == m.start {
					marker = ">"
				}
				fmt.Fprintf(&sb, "%s %4d │ %s\n", marker, absLine, line)
			}
			if ctxLines > 0 && len(matches) > 1 {
				sb.WriteString("  ···\n")
			}
		}
		return strings.TrimRight(sb.String(), "\n")
	},
}

// ─── append_file ─────────────────────────────────────────────────────────────

var AppendFile = &ToolDef{
	Name:        "append_file",
	Description: "Append text to an existing file (creates if not exists). Adds a newline separator if the file doesn't end with one.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "path", Description: "File path to append to", Required: true},
		{Name: "content", Description: "Content to append", Required: true},
	},
	Execute: func(args map[string]string) string {
		path := args["path"]
		if path == "" {
			return "Error: path is required"
		}
		content := sanitizeFileContent(args["content"])

		// Ensure separator newline if file exists and doesn't end with one
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			f, err := os.Open(path)
			if err == nil {
				buf := make([]byte, 1)
				f.Seek(-1, 2)
				f.Read(buf)
				f.Close()
				if buf[0] != '\n' {
					content = "\n" + content
				}
			}
		}

		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		defer f.Close()
		n, err := f.WriteString(content)
		if err != nil {
			return fmt.Sprintf("Error writing: %v", err)
		}
		return fmt.Sprintf("OK — appended %d bytes to %s", n, path)
	},
}

// ─── list_dir ─────────────────────────────────────────────────────────────────

var ListDir = &ToolDef{
	Name:        "list_dir",
	Description: "List files and directories at a given path. Set recursive=true for tree view.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "path", Description: "Directory path (defaults to current directory)", Required: false},
		{Name: "recursive", Description: "Show full tree (true/false, default: false)", Required: false},
	},
	Execute: func(args map[string]string) string {
		root := args["path"]
		if root == "" {
			root = "."
		}
		recursive := args["recursive"] == "true"

		if !recursive {
			entries, err := os.ReadDir(root)
			if err != nil {
				return fmt.Sprintf("Error: %v", err)
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "Contents of %s: (%d entries)\n", root, len(entries))
			for _, e := range entries {
				kind := "file"
				if e.IsDir() {
					kind = "dir "
				}
				info, _ := e.Info()
				size := ""
				if info != nil && !e.IsDir() {
					size = fmt.Sprintf(" (%s)", fmtSize(info.Size()))
				}
				fmt.Fprintf(&sb, "  [%s] %s%s\n", kind, e.Name(), size)
			}
			return strings.TrimSpace(sb.String())
		}

		var sb strings.Builder
		count := 0
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			if rel == "." {
				return nil
			}
			depth := strings.Count(rel, string(filepath.Separator))
			indent := strings.Repeat("  ", depth)
			name := d.Name()
			if d.IsDir() {
				fmt.Fprintf(&sb, "%s📁 %s/\n", indent, name)
			} else {
				info, _ := d.Info()
				size := ""
				if info != nil {
					size = " (" + fmtSize(info.Size()) + ")"
				}
				fmt.Fprintf(&sb, "%s📄 %s%s\n", indent, name, size)
			}
			count++
			if count > 300 {
				sb.WriteString("  ...truncated at 300 entries\n")
				return fs.SkipAll
			}
			return nil
		})
		return strings.TrimSpace(sb.String())
	},
}

func fmtSize(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

// ─── create_dir ───────────────────────────────────────────────────────────────

var CreateDir = &ToolDef{
	Name:        "create_dir",
	Description: "Create a directory (and any missing parent directories)",
	Secure:      true,
	Args: []ToolArg{
		{Name: "path", Description: "Directory path to create", Required: true},
	},
	Execute: func(args map[string]string) string {
		path := args["path"]
		if path == "" {
			return "Error: path is required"
		}
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return fmt.Sprintf("OK — directory created: %s", path)
	},
}

// ─── delete_file ──────────────────────────────────────────────────────────────

var DeleteFile = &ToolDef{
	Name:        "delete_file",
	Description: "Delete a file or an empty directory. Use recursive=true to delete a directory and all contents.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "path", Description: "File or directory path to delete", Required: true},
		{Name: "recursive", Description: "Delete directory recursively (true/false, default: false)", Required: false},
	},
	Execute: func(args map[string]string) string {
		path := args["path"]
		if path == "" {
			return "Error: path is required"
		}
		var err error
		if args["recursive"] == "true" {
			err = os.RemoveAll(path)
		} else {
			err = os.Remove(path)
		}
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return fmt.Sprintf("OK — deleted: %s", path)
	},
}

// ─── move_file ────────────────────────────────────────────────────────────────

var MoveFile = &ToolDef{
	Name:        "move_file",
	Description: "Move or rename a file or directory",
	Secure:      true,
	Args: []ToolArg{
		{Name: "src", Description: "Source path", Required: true},
		{Name: "dst", Description: "Destination path", Required: true},
	},
	Execute: func(args map[string]string) string {
		src := args["src"]
		dst := args["dst"]
		if src == "" || dst == "" {
			return "Error: both src and dst are required"
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Sprintf("Error creating destination dirs: %v", err)
		}
		if err := os.Rename(src, dst); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return fmt.Sprintf("OK — moved %s → %s", src, dst)
	},
}

// ─── search_files ─────────────────────────────────────────────────────────────

var SearchFiles = &ToolDef{
	Name:        "search_files",
	Description: "Search for files matching a name pattern recursively. Supports glob patterns like '*.go', '*config*', '**/*.json'.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "dir", Description: "Root directory to search in (defaults to current directory)", Required: false},
		{Name: "pattern", Description: "Glob pattern to match filenames (e.g. '*.go', '*test*')", Required: true},
		{Name: "max_results", Description: "Maximum results to return (default: 100)", Required: false},
	},
	Execute: func(args map[string]string) string {
		root := args["dir"]
		if root == "" {
			root = "."
		}
		pattern := args["pattern"]
		if pattern == "" {
			return "Error: pattern is required"
		}
		maxResults := 100
		if m := strings.TrimSpace(args["max_results"]); m != "" {
			if n, err := strconv.Atoi(m); err == nil && n > 0 {
				maxResults = n
			}
		}

		var matches []string
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || len(matches) >= maxResults {
				return nil
			}
			matched, _ := filepath.Match(pattern, d.Name())
			if matched {
				matches = append(matches, path)
			}
			return nil
		})

		if len(matches) == 0 {
			return fmt.Sprintf("No files found matching %q in %s", pattern, root)
		}
		suffix := ""
		if len(matches) == maxResults {
			suffix = fmt.Sprintf("\n(limited to %d results)", maxResults)
		}
		return fmt.Sprintf("Found %d match(es):\n%s%s", len(matches), strings.Join(matches, "\n"), suffix)
	},
}

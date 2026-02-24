package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ReadFile = &ToolDef{
	Name:        "read_file",
	Description: "Read the contents of a file on disk",
	Secure:      true,
	Args: []ToolArg{
		{Name: "path", Description: "Absolute or relative file path to read", Required: true},
	},
	Execute: func(args map[string]string) string {
		path := args["path"]
		if path == "" {
			return "Error: path is required"
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		content := string(data)
		if len(content) > 8000 {
			content = content[:8000] + "\n...(truncated at 8000 chars)"
		}
		return content
	},
}

var WriteFile = &ToolDef{
	Name:        "write_file",
	Description: "Write or create a file with the given content",
	Secure:      true,
	Args: []ToolArg{
		{Name: "path", Description: "File path to write to", Required: true},
		{Name: "content", Description: "Content to write into the file", Required: true},
	},
	Execute: func(args map[string]string) string {
		path := args["path"]
		content := args["content"]
		if path == "" {
			return "Error: path is required"
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Sprintf("Error creating directories: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return fmt.Sprintf("OK — wrote %d bytes to %s", len(content), path)
	},
}

var AppendFile = &ToolDef{
	Name:        "append_file",
	Description: "Append text to an existing file (creates the file if it doesn't exist)",
	Secure:      true,
	Args: []ToolArg{
		{Name: "path", Description: "File path to append to", Required: true},
		{Name: "content", Description: "Content to append", Required: true},
	},
	Execute: func(args map[string]string) string {
		path := args["path"]
		content := args["content"]
		if path == "" {
			return "Error: path is required"
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

var ListDir = &ToolDef{
	Name:        "list_dir",
	Description: "List files and directories at a given path",
	Secure:      true,
	Args: []ToolArg{
		{Name: "path", Description: "Directory path to list (defaults to current directory)", Required: false},
	},
	Execute: func(args map[string]string) string {
		path := args["path"]
		if path == "" {
			path = "."
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Contents of %s:\n", path))
		for _, e := range entries {
			kind := "file"
			if e.IsDir() {
				kind = "dir "
			}
			info, _ := e.Info()
			size := ""
			if info != nil && !e.IsDir() {
				size = fmt.Sprintf(" (%d bytes)", info.Size())
			}
			sb.WriteString(fmt.Sprintf("  [%s] %s%s\n", kind, e.Name(), size))
		}
		return strings.TrimSpace(sb.String())
	},
}

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

var DeleteFile = &ToolDef{
	Name:        "delete_file",
	Description: "Delete a file or an empty directory. Use recursive=true to delete a directory and all contents.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "path", Description: "File or directory path to delete", Required: true},
		{Name: "recursive", Description: "Set to 'true' to delete directories and all their contents", Required: false},
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

var SearchFiles = &ToolDef{
	Name:        "search_files",
	Description: "Search for files matching a name pattern recursively within a directory. Supports glob patterns like '*.go' or '*test*'.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "dir", Description: "Root directory to search in (defaults to current directory)", Required: false},
		{Name: "pattern", Description: "Glob pattern to match filenames (e.g. '*.go', '*config*')", Required: true},
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
		var matches []string
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			matched, _ := filepath.Match(pattern, info.Name())
			if matched {
				matches = append(matches, path)
			}
			return nil
		})
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		if len(matches) == 0 {
			return fmt.Sprintf("No files found matching %q in %s", pattern, root)
		}
		if len(matches) > 100 {
			matches = matches[:100]
		}
		return fmt.Sprintf("Found %d matches:\n%s", len(matches), strings.Join(matches, "\n"))
	},
}

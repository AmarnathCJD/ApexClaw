package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"apexclaw/model"
)

type CustomTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Args        string `json:"args"`
	Code        string `json:"code"`
	Language    string `json:"language"`
	CreatedAt   string `json:"created_at"`
	RunCount    int    `json:"run_count"`
	LastRun     string `json:"last_run"`
}

var (
	customToolsMu    sync.Mutex
	customToolsCache = make(map[string]*CustomTool)
)

var CustomToolRegisterFn func(name, description, argsJSON, code, language string)

func customToolsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".apexclaw", "custom_tools")
}

func customToolsIndexPath() string {
	return filepath.Join(customToolsDir(), "index.json")
}

func loadCustomTools() map[string]*CustomTool {
	customToolsMu.Lock()
	defer customToolsMu.Unlock()
	data, err := os.ReadFile(customToolsIndexPath())
	if err != nil {
		return make(map[string]*CustomTool)
	}
	var tools map[string]*CustomTool
	if err := json.Unmarshal(data, &tools); err != nil {
		return make(map[string]*CustomTool)
	}
	customToolsCache = tools
	return tools
}

func saveCustomTools() {
	customToolsMu.Lock()
	defer customToolsMu.Unlock()
	os.MkdirAll(customToolsDir(), 0755)
	data, _ := json.MarshalIndent(customToolsCache, "", "  ")
	os.WriteFile(customToolsIndexPath(), data, 0644)
}

var ToolCreate = &ToolDef{
	Name:        "tool_create",
	Description: "Create a new custom tool that gets immediately available in this session. Describe what you want, and the AI writes and registers it. The tool persists across restarts.",
	Args: []ToolArg{
		{Name: "name", Description: "Tool name in snake_case (e.g. 'fetch_crypto_price')", Required: true},
		{Name: "description", Description: "What this tool does (shown to AI when choosing tools)", Required: true},
		{Name: "args", Description: "JSON array of args: [{\"name\":\"x\",\"description\":\"...\",\"required\":true}]", Required: false},
		{Name: "code", Description: "Python code for the tool body. Receives args as a dict called 'args'. Must print the result. Leave empty to auto-generate.", Required: false},
		{Name: "task", Description: "Natural language description of what the tool should do (used for auto-generation if code is empty)", Required: false},
	},
	Execute: func(args map[string]string) string {
		name := args["name"]
		description := args["description"]
		argsJSON := args["args"]
		code := args["code"]
		task := args["task"]

		if name == "" || description == "" {
			return "Error: name and description are required"
		}
		name = strings.ToLower(strings.ReplaceAll(name, " ", "_"))
		if argsJSON == "" {
			argsJSON = "[]"
		}

		if code == "" {
			if task == "" {
				task = description
			}
			var err error
			code, err = generateToolCode(name, description, argsJSON, task)
			if err != nil {
				return fmt.Sprintf("Error generating tool code: %v", err)
			}
		}

		ct := &CustomTool{
			Name:        name,
			Description: description,
			Args:        argsJSON,
			Code:        code,
			Language:    "python",
			CreatedAt:   time.Now().Format(time.RFC3339),
		}

		os.MkdirAll(customToolsDir(), 0755)
		codeFile := filepath.Join(customToolsDir(), name+".py")
		if err := os.WriteFile(codeFile, []byte(code), 0644); err != nil {
			return fmt.Sprintf("Error saving tool code: %v", err)
		}

		customToolsMu.Lock()
		customToolsCache[name] = ct
		customToolsMu.Unlock()
		saveCustomTools()

		if CustomToolRegisterFn != nil {
			CustomToolRegisterFn(name, description, argsJSON, code, "python")
		}

		return fmt.Sprintf("Tool %q created and registered!\n\nCode:\n```python\n%s\n```\n\nYou can now call it with: %s", name, code, name)
	},
}

func generateToolCode(name, description, argsJSON, task string) (string, error) {
	client := model.New()

	prompt := fmt.Sprintf(`Write a Python tool function for an AI assistant.

Tool name: %s
Description: %s
Args spec (JSON): %s
Task: %s

Requirements:
1. The code receives a dict called 'args' with the tool arguments
2. Must print the result (the last print() is captured as the tool output)
3. Keep it concise and practical
4. Handle errors gracefully with try/except
5. Use only Python stdlib unless the task absolutely needs a package
6. Return useful, structured output

Write ONLY the Python code, no markdown fences, no explanations.`, name, description, argsJSON, task)

	messages := []model.Message{{Role: "user", Content: prompt}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reply, err := client.Send(ctx, "claude-sonnet-4-6", messages)
	if err != nil {
		return "", err
	}
	code := strings.TrimSpace(reply)
	code = strings.TrimPrefix(code, "```python")
	code = strings.TrimPrefix(code, "```")
	code = strings.TrimSuffix(code, "```")
	return strings.TrimSpace(code), nil
}

var ToolListCustom = &ToolDef{
	Name:        "tool_list_custom",
	Description: "List all custom tools created with tool_create, showing their descriptions and run counts.",
	Args:        []ToolArg{},
	Execute: func(args map[string]string) string {
		tools := loadCustomTools()
		if len(tools) == 0 {
			return "No custom tools created yet. Use tool_create to build new tools."
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Custom Tools (%d)\n\n", len(tools))
		for _, t := range tools {
			fmt.Fprintf(&sb, "• %s — %s\n  Created: %s | Runs: %d\n", t.Name, t.Description, t.CreatedAt[:10], t.RunCount)
		}
		return strings.TrimRight(sb.String(), "\n")
	},
}

var ToolDeleteCustom = &ToolDef{
	Name:        "tool_delete",
	Description: "Delete a custom tool by name.",
	Args: []ToolArg{
		{Name: "name", Description: "The tool name to delete", Required: true},
	},
	Execute: func(args map[string]string) string {
		name := args["name"]
		if name == "" {
			return "Error: name is required"
		}
		customToolsMu.Lock()
		_, ok := customToolsCache[name]
		if ok {
			delete(customToolsCache, name)
		}
		customToolsMu.Unlock()
		if !ok {
			return fmt.Sprintf("No custom tool found with name %q.", name)
		}
		os.Remove(filepath.Join(customToolsDir(), name+".py"))
		saveCustomTools()
		return fmt.Sprintf("Custom tool %q deleted.", name)
	},
}

var ToolRunCustom = &ToolDef{
	Name:        "tool_run",
	Description: "Execute a custom tool created with tool_create by name, passing arguments as JSON.",
	Args: []ToolArg{
		{Name: "name", Description: "The custom tool name to run", Required: true},
		{Name: "args_json", Description: "JSON object of arguments to pass, e.g. {\"url\": \"https://...\"}", Required: false},
	},
	Execute: func(args map[string]string) string {
		name := args["name"]
		argsJSON := args["args_json"]
		if name == "" {
			return "Error: name is required"
		}
		if argsJSON == "" {
			argsJSON = "{}"
		}

		tools := loadCustomTools()
		ct, ok := tools[name]
		if !ok {
			return fmt.Sprintf("No custom tool %q found. Use tool_list_custom to see available tools.", name)
		}

		codeFile := filepath.Join(customToolsDir(), name+".py")
		if _, err := os.Stat(codeFile); err != nil {
			return fmt.Sprintf("Tool code file missing: %s", codeFile)
		}

		runner := fmt.Sprintf(`import json, sys
args = json.loads(r'''%s''')
%s
`, argsJSON, ct.Code)

		tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("claw_tool_%s_%d.py", name, time.Now().UnixNano()))
		if err := os.WriteFile(tmpFile, []byte(runner), 0644); err != nil {
			return fmt.Sprintf("Error creating runner: %v", err)
		}
		defer os.Remove(tmpFile)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "python3", tmpFile)
		out, err := cmd.CombinedOutput()

		customToolsMu.Lock()
		if t, ok := customToolsCache[name]; ok {
			t.RunCount++
			t.LastRun = time.Now().Format(time.RFC3339)
		}
		customToolsMu.Unlock()
		go saveCustomTools()

		if err != nil {
			return fmt.Sprintf("Tool error: %v\n%s", err, string(out))
		}
		return strings.TrimSpace(string(out))
	},
}

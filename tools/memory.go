package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var memoryMu sync.Mutex

func memoryPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".apexclaw", "memory.json")
}

func loadMemory() map[string]map[string]string {
	memoryMu.Lock()
	defer memoryMu.Unlock()
	data, err := os.ReadFile(memoryPath())
	if err != nil {
		return make(map[string]map[string]string)
	}
	var m map[string]map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return make(map[string]map[string]string)
	}
	return m
}

func saveMemory(m map[string]map[string]string) error {
	memoryMu.Lock()
	defer memoryMu.Unlock()
	path := memoryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

var SaveFact = &ToolDef{
	Name:        "save_fact",
	Description: "Save a persistent fact or piece of information to long-term memory. Use a clear key like 'user_name' or 'project_deadline'.",
	Args: []ToolArg{
		{Name: "key", Description: "Unique identifier for the fact (e.g. 'user_timezone')", Required: true},
		{Name: "value", Description: "Value or note to store", Required: true},
		{Name: "category", Description: "Optional category to group related facts (e.g. 'preferences', 'tasks'). Defaults to 'general'.", Required: false},
	},
	Execute: func(args map[string]string) string {
		key := args["key"]
		value := args["value"]
		if key == "" || value == "" {
			return "Error: key and value are required"
		}
		category := args["category"]
		if category == "" {
			category = "general"
		}
		m := loadMemory()
		if m[category] == nil {
			m[category] = make(map[string]string)
		}
		m[category][key] = value
		if err := saveMemory(m); err != nil {
			return fmt.Sprintf("Error saving: %v", err)
		}
		return fmt.Sprintf("Saved: [%s] %s = %q", category, key, value)
	},
}

var RecallFact = &ToolDef{
	Name:        "recall_fact",
	Description: "Recall a specific fact from long-term memory by key.",
	Args: []ToolArg{
		{Name: "key", Description: "Key to look up", Required: true},
		{Name: "category", Description: "Category to search in. Defaults to 'general'. Use 'all' to search everywhere.", Required: false},
	},
	Execute: func(args map[string]string) string {
		key := args["key"]
		if key == "" {
			return "Error: key is required"
		}
		category := args["category"]
		m := loadMemory()

		if category == "all" || category == "" {
			for cat, facts := range m {
				if val, ok := facts[key]; ok {
					return fmt.Sprintf("[%s] %s = %q", cat, key, val)
				}
			}
			return fmt.Sprintf("No fact found for key %q", key)
		}

		if m[category] == nil {
			return fmt.Sprintf("No category %q found", category)
		}
		val, ok := m[category][key]
		if !ok {
			return fmt.Sprintf("No fact found for key %q in category %q", key, category)
		}
		return fmt.Sprintf("[%s] %s = %q", category, key, val)
	},
}

var ListFacts = &ToolDef{
	Name:        "list_facts",
	Description: "List all stored facts, optionally filtered by category.",
	Args: []ToolArg{
		{Name: "category", Description: "Category to list (optional). Leave blank for all.", Required: false},
	},
	Execute: func(args map[string]string) string {
		category := args["category"]
		m := loadMemory()
		if len(m) == 0 {
			return "No facts stored yet."
		}
		var sb strings.Builder
		for cat, facts := range m {
			if category != "" && cat != category {
				continue
			}
			sb.WriteString(fmt.Sprintf("[%s]\n", cat))
			for k, v := range facts {
				sb.WriteString(fmt.Sprintf("  %s = %q\n", k, v))
			}
		}
		out := strings.TrimSpace(sb.String())
		if out == "" {
			return fmt.Sprintf("No facts found in category %q", category)
		}
		return out
	},
}

var DeleteFact = &ToolDef{
	Name:        "delete_fact",
	Description: "Delete a specific fact from memory by key and category.",
	Args: []ToolArg{
		{Name: "key", Description: "Key to delete", Required: true},
		{Name: "category", Description: "Category the key belongs to. Defaults to 'general'.", Required: false},
	},
	Execute: func(args map[string]string) string {
		key := args["key"]
		if key == "" {
			return "Error: key is required"
		}
		category := args["category"]
		if category == "" {
			category = "general"
		}
		m := loadMemory()
		if m[category] == nil {
			return fmt.Sprintf("No category %q found", category)
		}
		if _, ok := m[category][key]; !ok {
			return fmt.Sprintf("Key %q not found in [%s]", key, category)
		}
		delete(m[category], key)
		if len(m[category]) == 0 {
			delete(m, category)
		}
		if err := saveMemory(m); err != nil {
			return fmt.Sprintf("Error saving: %v", err)
		}
		return fmt.Sprintf("Deleted: [%s] %s", category, key)
	},
}

var UpdateNote = &ToolDef{
	Name:        "update_note",
	Description: "Append a timestamped entry to a named note (like a running log or journal). Great for tracking tasks, decisions, or events over time.",
	Args: []ToolArg{
		{Name: "note", Description: "Note name/identifier (e.g. 'daily_log', 'task_list')", Required: true},
		{Name: "entry", Description: "Text to append to the note", Required: true},
	},
	Execute: func(args map[string]string) string {
		note := args["note"]
		entry := args["entry"]
		if note == "" || entry == "" {
			return "Error: note and entry are required"
		}
		ts := time.Now().Format("2006-01-02 15:04:05")
		m := loadMemory()
		if m["notes"] == nil {
			m["notes"] = make(map[string]string)
		}
		existing := m["notes"][note]
		if existing != "" {
			m["notes"][note] = existing + fmt.Sprintf("\n[%s] %s", ts, entry)
		} else {
			m["notes"][note] = fmt.Sprintf("[%s] %s", ts, entry)
		}
		if err := saveMemory(m); err != nil {
			return fmt.Sprintf("Error saving note: %v", err)
		}
		return fmt.Sprintf("Note %q updated.", note)
	},
}

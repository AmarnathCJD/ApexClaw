package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type todoItem struct {
	ID        int    `json:"id"`
	Text      string `json:"text"`
	Done      bool   `json:"done"`
	CreatedAt string `json:"created_at"`
	DoneAt    string `json:"done_at,omitempty"`
	Tag       string `json:"tag,omitempty"`
}

type todoStore struct {
	mu     sync.Mutex
	items  []todoItem
	nextID int
}

var todos = &todoStore{}

func todoPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".apexclaw", "todos.json")
}

func (s *todoStore) load() {
	data, err := os.ReadFile(todoPath())
	if err != nil {
		return
	}
	var items []todoItem
	if err := json.Unmarshal(data, &items); err != nil {
		return
	}
	s.items = items
	for _, it := range items {
		if it.ID >= s.nextID {
			s.nextID = it.ID + 1
		}
	}
}

func (s *todoStore) save() {
	path := todoPath()
	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(s.items, "", "  ")
	_ = os.WriteFile(path, data, 0644)
}

func init() {
	todos.load()
}

var TodoAdd = &ToolDef{
	Name:        "todo_add",
	Description: "Add a new item to the persistent todo list.",
	Args: []ToolArg{
		{Name: "text", Description: "The todo item text", Required: true},
		{Name: "tag", Description: "Optional tag/category (e.g. 'work', 'personal', 'shopping')", Required: false},
	},
	Execute: func(args map[string]string) string {
		text := strings.TrimSpace(args["text"])
		if text == "" {
			return "Error: text is required"
		}
		tag := strings.TrimSpace(args["tag"])

		todos.mu.Lock()
		id := todos.nextID
		todos.nextID++
		item := todoItem{
			ID:        id,
			Text:      text,
			Done:      false,
			CreatedAt: time.Now().Format("02 Jan 15:04"),
			Tag:       tag,
		}
		todos.items = append(todos.items, item)
		todos.save()
		todos.mu.Unlock()

		tagStr := ""
		if tag != "" {
			tagStr = fmt.Sprintf(" [%s]", tag)
		}
		return fmt.Sprintf("Added todo #%d: %s%s", id, text, tagStr)
	},
}

var TodoList = &ToolDef{
	Name:        "todo_list",
	Description: "List all todo items, optionally filtered by tag or status.",
	Args: []ToolArg{
		{Name: "filter", Description: "Filter by: 'pending' (default), 'done', 'all', or a tag name", Required: false},
	},
	Execute: func(args map[string]string) string {
		filter := strings.ToLower(strings.TrimSpace(args["filter"]))
		if filter == "" {
			filter = "pending"
		}

		todos.mu.Lock()
		items := make([]todoItem, len(todos.items))
		copy(items, todos.items)
		todos.mu.Unlock()

		var filtered []todoItem
		for _, it := range items {
			switch filter {
			case "pending":
				if !it.Done {
					filtered = append(filtered, it)
				}
			case "done":
				if it.Done {
					filtered = append(filtered, it)
				}
			case "all":
				filtered = append(filtered, it)
			default:

				if strings.EqualFold(it.Tag, filter) {
					filtered = append(filtered, it)
				}
			}
		}

		if len(filtered) == 0 {
			if filter == "pending" {
				return "No pending todos. You're all caught up!"
			}
			return fmt.Sprintf("No todos matching filter %q.", filter)
		}

		var sb strings.Builder
		label := map[string]string{
			"pending": "Pending",
			"done":    "Completed",
			"all":     "All",
		}[filter]
		if label == "" {
			label = fmt.Sprintf("Tag: %s", filter)
		}
		sb.WriteString(fmt.Sprintf("📋 %s todos (%d):\n\n", label, len(filtered)))
		for _, it := range filtered {
			status := "[ ]"
			if it.Done {
				status = "[x]"
			}
			tagStr := ""
			if it.Tag != "" {
				tagStr = fmt.Sprintf(" [%s]", it.Tag)
			}
			sb.WriteString(fmt.Sprintf("%s #%d %s%s\n    Added: %s", status, it.ID, it.Text, tagStr, it.CreatedAt))
			if it.Done && it.DoneAt != "" {
				sb.WriteString(fmt.Sprintf(" | Done: %s", it.DoneAt))
			}
			sb.WriteString("\n")
		}
		return strings.TrimRight(sb.String(), "\n")
	},
}

var TodoDone = &ToolDef{
	Name:        "todo_done",
	Description: "Mark todo item(s) as completed by ID.",
	Args: []ToolArg{
		{Name: "ids", Description: "Comma-separated todo IDs to mark done (e.g. '1' or '1,3,5')", Required: true},
	},
	Execute: func(args map[string]string) string {
		idsStr := strings.TrimSpace(args["ids"])
		if idsStr == "" {
			return "Error: ids is required"
		}
		var ids []int
		for _, s := range strings.Split(idsStr, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			id, err := strconv.Atoi(s)
			if err != nil {
				return fmt.Sprintf("Error: invalid ID %q", s)
			}
			ids = append(ids, id)
		}

		todos.mu.Lock()
		now := time.Now().Format("02 Jan 15:04")
		var done []string
		for i, it := range todos.items {
			for _, id := range ids {
				if it.ID == id && !it.Done {
					todos.items[i].Done = true
					todos.items[i].DoneAt = now
					done = append(done, fmt.Sprintf("#%d %s", it.ID, it.Text))
				}
			}
		}
		todos.save()
		todos.mu.Unlock()

		if len(done) == 0 {
			return "No matching pending todos found."
		}
		return "Marked done: " + strings.Join(done, ", ")
	},
}

var TodoDelete = &ToolDef{
	Name:        "todo_delete",
	Description: "Delete todo item(s) by ID, or delete all completed todos.",
	Args: []ToolArg{
		{Name: "ids", Description: "Comma-separated IDs to delete, or 'done' to clear all completed items", Required: true},
	},
	Execute: func(args map[string]string) string {
		idsStr := strings.TrimSpace(args["ids"])
		if idsStr == "" {
			return "Error: ids is required"
		}

		todos.mu.Lock()
		defer todos.mu.Unlock()

		if strings.EqualFold(idsStr, "done") {
			before := len(todos.items)
			var remaining []todoItem
			for _, it := range todos.items {
				if !it.Done {
					remaining = append(remaining, it)
				}
			}
			removed := before - len(remaining)
			todos.items = remaining
			todos.save()
			return fmt.Sprintf("Cleared %d completed todo(s).", removed)
		}

		var ids []int
		for _, s := range strings.Split(idsStr, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			id, err := strconv.Atoi(s)
			if err != nil {
				return fmt.Sprintf("Error: invalid ID %q", s)
			}
			ids = append(ids, id)
		}

		idSet := map[int]bool{}
		for _, id := range ids {
			idSet[id] = true
		}
		var remaining []todoItem
		var deleted []string
		for _, it := range todos.items {
			if idSet[it.ID] {
				deleted = append(deleted, fmt.Sprintf("#%d", it.ID))
			} else {
				remaining = append(remaining, it)
			}
		}
		todos.items = remaining
		todos.save()

		if len(deleted) == 0 {
			return "No matching todos found."
		}
		return "Deleted: " + strings.Join(deleted, ", ")
	},
}

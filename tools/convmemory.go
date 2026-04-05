package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"apexclaw/model"
)

type MemoryFact struct {
	ID         string   `json:"id"`
	Content    string   `json:"content"`
	Category   string   `json:"category"`
	Tags       []string `json:"tags"`
	Source     string   `json:"source"`
	Confidence float64  `json:"confidence"`
	CreatedAt  string   `json:"created_at"`
	LastUsed   string   `json:"last_used"`
	UseCount   int      `json:"use_count"`
	OwnerID    string   `json:"owner_id"`
}

type memoryStore struct {
	mu    sync.Mutex
	facts map[string]map[string]*MemoryFact
}

var memStore = &memoryStore{facts: make(map[string]map[string]*MemoryFact)}

func convMemoryPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".apexclaw", "conv_memory.json")
}

func loadConvMemory() {
	memStore.mu.Lock()
	defer memStore.mu.Unlock()
	data, err := os.ReadFile(convMemoryPath())
	if err != nil {
		return
	}
	json.Unmarshal(data, &memStore.facts)
}

func saveConvMemory() {
	memStore.mu.Lock()
	defer memStore.mu.Unlock()
	path := convMemoryPath()
	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(memStore.facts, "", "  ")
	os.WriteFile(path, data, 0644)
}

func InitMemory() {
	loadConvMemory()
}

var MemoryExtract = &ToolDef{
	Name:        "memory_extract",
	Description: "Extract and store important facts, preferences, or context from a conversation or text. The AI identifies key information and saves it for future recall.",
	Args: []ToolArg{
		{Name: "text", Description: "The conversation or text to extract facts from", Required: true},
		{Name: "category", Description: "Category hint: 'preference', 'fact', 'task', 'context', 'person', 'auto' (default: auto)", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		text := args["text"]
		if text == "" {
			return "Error: text is required"
		}
		category := args["category"]
		if category == "" {
			category = "auto"
		}

		ownerID := userID
		if GetTelegramContextFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				if v, ok := ctx["owner_id"].(string); ok && v != "" {
					ownerID = v
				}
			}
		}

		facts, err := extractFactsWithLLM(text, category)
		if err != nil {
			return fmt.Sprintf("Error extracting facts: %v", err)
		}
		if len(facts) == 0 {
			return "No significant facts found to extract."
		}

		memStore.mu.Lock()
		if memStore.facts[ownerID] == nil {
			memStore.facts[ownerID] = make(map[string]*MemoryFact)
		}
		saved := 0
		for _, f := range facts {
			f.OwnerID = ownerID
			if f.ID == "" {
				f.ID = fmt.Sprintf("mem_%d_%d", time.Now().UnixNano(), saved)
			}
			memStore.facts[ownerID][f.ID] = f
			saved++
		}
		memStore.mu.Unlock()
		go saveConvMemory()

		var sb strings.Builder
		fmt.Fprintf(&sb, "Extracted and stored %d facts:\n\n", saved)
		for _, f := range facts {
			cat := f.Category
			if cat == "" {
				cat = "fact"
			}
			fmt.Fprintf(&sb, "• [%s] %s\n", cat, f.Content)
		}
		return strings.TrimRight(sb.String(), "\n")
	},
	Execute: func(args map[string]string) string {
		return "Error: memory_extract requires context"
	},
}

var MemoryRecall = &ToolDef{
	Name:        "memory_recall",
	Description: "Search and recall stored memories relevant to a query. Returns the most relevant facts from past conversations.",
	Args: []ToolArg{
		{Name: "query", Description: "What to search for in memory", Required: true},
		{Name: "category", Description: "Filter by category: 'preference', 'fact', 'task', 'context', 'person'", Required: false},
		{Name: "limit", Description: "Max results to return (default: 10)", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		query := args["query"]
		if query == "" {
			return "Error: query is required"
		}
		category := args["category"]
		limitStr := args["limit"]
		limit := 10
		if limitStr != "" {
			fmt.Sscanf(limitStr, "%d", &limit)
		}

		ownerID := userID
		if GetTelegramContextFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				if v, ok := ctx["owner_id"].(string); ok && v != "" {
					ownerID = v
				}
			}
		}

		memStore.mu.Lock()
		userFacts := memStore.facts[ownerID]
		if len(userFacts) == 0 {
			memStore.mu.Unlock()
			return "No memories stored yet. Use memory_extract to save facts from conversations."
		}
		allFacts := make([]*MemoryFact, 0, len(userFacts))
		for _, f := range userFacts {
			if category != "" && f.Category != category {
				continue
			}
			allFacts = append(allFacts, f)
		}
		memStore.mu.Unlock()

		scored := scoreMemoryFacts(allFacts, query)
		if len(scored) == 0 {
			return "No relevant memories found for that query."
		}
		if len(scored) > limit {
			scored = scored[:limit]
		}

		now := time.Now().Format(time.RFC3339)
		memStore.mu.Lock()
		for _, f := range scored {
			if mf, ok := memStore.facts[ownerID][f.ID]; ok {
				mf.LastUsed = now
				mf.UseCount++
			}
		}
		memStore.mu.Unlock()
		go saveConvMemory()

		var sb strings.Builder
		fmt.Fprintf(&sb, "Recalled %d relevant memories:\n\n", len(scored))
		for i, f := range scored {
			cat := f.Category
			if cat == "" {
				cat = "fact"
			}
			fmt.Fprintf(&sb, "%d. [%s] %s", i+1, cat, f.Content)
			if f.CreatedAt != "" && len(f.CreatedAt) >= 10 {
				fmt.Fprintf(&sb, " _(stored %s)_", f.CreatedAt[:10])
			}
			fmt.Fprintln(&sb)
		}
		return strings.TrimRight(sb.String(), "\n")
	},
	Execute: func(args map[string]string) string {
		return "Error: requires context"
	},
}

var MemoryForget = &ToolDef{
	Name:        "memory_forget",
	Description: "Delete specific memories or clear all memories. Use to remove outdated or incorrect facts.",
	Args: []ToolArg{
		{Name: "id", Description: "Specific memory ID to delete (from memory_recall)", Required: false},
		{Name: "category", Description: "Delete all memories of a category", Required: false},
		{Name: "all", Description: "Set to 'true' to clear all your memories", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		ownerID := userID
		if GetTelegramContextFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				if v, ok := ctx["owner_id"].(string); ok && v != "" {
					ownerID = v
				}
			}
		}

		if args["all"] == "true" {
			memStore.mu.Lock()
			count := len(memStore.facts[ownerID])
			delete(memStore.facts, ownerID)
			memStore.mu.Unlock()
			go saveConvMemory()
			return fmt.Sprintf("Cleared all %d memories.", count)
		}

		if id := args["id"]; id != "" {
			memStore.mu.Lock()
			_, ok := memStore.facts[ownerID][id]
			if ok {
				delete(memStore.facts[ownerID], id)
			}
			memStore.mu.Unlock()
			if !ok {
				return fmt.Sprintf("Memory %q not found.", id)
			}
			go saveConvMemory()
			return fmt.Sprintf("Memory %q deleted.", id)
		}

		if cat := args["category"]; cat != "" {
			memStore.mu.Lock()
			count := 0
			for id, f := range memStore.facts[ownerID] {
				if f.Category == cat {
					delete(memStore.facts[ownerID], id)
					count++
				}
			}
			memStore.mu.Unlock()
			go saveConvMemory()
			return fmt.Sprintf("Deleted %d memories in category %q.", count, cat)
		}

		return "Error: specify id, category, or all=true"
	},
	Execute: func(args map[string]string) string {
		return "Error: requires context"
	},
}

var MemoryStats = &ToolDef{
	Name:        "memory_stats",
	Description: "Show memory statistics: how many facts are stored, categories breakdown, and most-used memories.",
	Args:        []ToolArg{},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		ownerID := userID
		if GetTelegramContextFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				if v, ok := ctx["owner_id"].(string); ok && v != "" {
					ownerID = v
				}
			}
		}

		memStore.mu.Lock()
		userFacts := memStore.facts[ownerID]
		memStore.mu.Unlock()

		if len(userFacts) == 0 {
			return "No memories stored yet."
		}

		cats := make(map[string]int)
		var topUsed []*MemoryFact
		for _, f := range userFacts {
			cats[f.Category]++
			topUsed = append(topUsed, f)
		}
		sort.Slice(topUsed, func(i, j int) bool {
			return topUsed[i].UseCount > topUsed[j].UseCount
		})

		var sb strings.Builder
		fmt.Fprintf(&sb, "Memory Stats\n\nTotal facts: %d\n\nBy category:\n", len(userFacts))
		for cat, count := range cats {
			if cat == "" {
				cat = "uncategorized"
			}
			fmt.Fprintf(&sb, "  %s: %d\n", cat, count)
		}
		if len(topUsed) > 0 {
			fmt.Fprintf(&sb, "\nMost recalled:\n")
			limit := 5
			if len(topUsed) < limit {
				limit = len(topUsed)
			}
			for _, f := range topUsed[:limit] {
				fmt.Fprintf(&sb, "  (%dx) %s\n", f.UseCount, f.Content)
			}
		}
		return strings.TrimRight(sb.String(), "\n")
	},
	Execute: func(args map[string]string) string {
		return "Error: requires context"
	},
}

func extractFactsWithLLM(text, category string) ([]*MemoryFact, error) {
	catHint := ""
	if category != "auto" && category != "" {
		catHint = fmt.Sprintf("Focus on extracting facts of category: %s\n", category)
	}

	prompt := fmt.Sprintf(`Extract important, reusable facts from this conversation or text that would be useful to remember for future interactions.

%sText:
%s

Return a JSON array of facts. Each fact:
{
  "id": "mem_<unique_short_id>",
  "content": "clear, concise statement of the fact",
  "category": "preference|fact|task|context|person|habit",
  "tags": ["tag1", "tag2"],
  "confidence": 0.0-1.0
}

Rules:
- Only extract genuinely useful, stable facts (not ephemeral chat)
- Preferences: what the user likes/dislikes/wants
- Facts: objective info about the user, their setup, projects
- Persons: info about people the user mentions
- Context: situational info that matters for future help
- Skip obvious, trivial, or one-time things
- Max 10 facts
- Return ONLY the JSON array, no markdown

If nothing worth storing, return []`, catHint, text)

	client := model.New()
	messages := []model.Message{{Role: "user", Content: prompt}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	reply, err := client.Send(ctx, "claude-sonnet-4-6", messages)
	if err != nil {
		return nil, err
	}

	content := strings.TrimSpace(reply.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var facts []*MemoryFact
	if err := json.Unmarshal([]byte(content), &facts); err != nil {
		return nil, fmt.Errorf("parse error: %v (raw: %s)", err, content[:min(len(content), 200)])
	}
	now := time.Now().Format(time.RFC3339)
	for _, f := range facts {
		f.CreatedAt = now
	}
	return facts, nil
}

func scoreMemoryFacts(facts []*MemoryFact, query string) []*MemoryFact {
	queryLower := strings.ToLower(query)
	queryWords := strings.Fields(queryLower)

	type scored struct {
		fact  *MemoryFact
		score float64
	}
	var results []scored

	for _, f := range facts {
		contentLower := strings.ToLower(f.Content)
		score := 0.0
		for _, w := range queryWords {
			if len(w) < 3 {
				continue
			}
			if strings.Contains(contentLower, w) {
				score += 1.0
			}
		}
		for _, tag := range f.Tags {
			if strings.Contains(strings.ToLower(tag), queryLower) {
				score += 0.5
			}
		}
		if strings.Contains(strings.ToLower(f.Category), queryLower) {
			score += 0.3
		}
		score += float64(f.UseCount) * 0.1
		if score > 0 {
			results = append(results, scored{f, score})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	out := make([]*MemoryFact, len(results))
	for i, r := range results {
		out[i] = r.fact
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

package core

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

const (
	cacheMaxEntries = 50
	cacheTTL        = 5 * time.Minute
)

// cacheableTools is an allowlist of tools whose results are safe to cache
// for a few minutes. Only deterministic, read-only, externally-expensive tools
// belong here. Anything that has side effects or non-deterministic output
// (chat sends, image gen, agent runs, file writes) MUST be excluded.
var cacheableTools = map[string]bool{
	"web_fetch":  true,
	"web_search": true,
}

type cacheEntry struct {
	key    string
	result string
	stored time.Time
}

// ToolCache is a per-AgentSession LRU + TTL cache of tool results.
type ToolCache struct {
	mu    sync.Mutex
	items map[string]*list.Element
	lru   *list.List
}

func NewToolCache() *ToolCache {
	return &ToolCache{
		items: make(map[string]*list.Element),
		lru:   list.New(),
	}
}

func isCacheable(toolName string) bool { return cacheableTools[toolName] }

// cacheKey produces a deterministic key. Round-trip through json.Unmarshal +
// json.Marshal — Go's encoder sorts map keys recursively, so identical args
// in different declaration orders hash the same.
func cacheKey(toolName, argsJSON string) string {
	var v any
	if err := json.Unmarshal([]byte(argsJSON), &v); err != nil {
		sum := sha256.Sum256([]byte(argsJSON))
		return toolName + "|raw|" + hex.EncodeToString(sum[:])
	}
	if m, ok := v.(map[string]any); ok {
		delete(m, "no_cache") // strip the bypass flag before hashing
	}
	canon, _ := json.Marshal(v)
	sum := sha256.Sum256(canon)
	return toolName + "|" + hex.EncodeToString(sum[:])
}

func bypassRequested(argsJSON string) bool {
	var m map[string]any
	if json.Unmarshal([]byte(argsJSON), &m) != nil {
		return false
	}
	b, _ := m["no_cache"].(bool)
	return b
}

// Get returns a cached result if present and unexpired. The hit return
// distinguishes cache miss from a real empty string.
func (c *ToolCache) Get(toolName, argsJSON string) (string, bool) {
	if !isCacheable(toolName) || bypassRequested(argsJSON) {
		return "", false
	}
	key := cacheKey(toolName, argsJSON)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sweepExpired()
	el, ok := c.items[key]
	if !ok {
		return "", false
	}
	ent := el.Value.(*cacheEntry)
	if time.Since(ent.stored) > cacheTTL {
		c.lru.Remove(el)
		delete(c.items, key)
		return "", false
	}
	c.lru.MoveToFront(el)
	return ent.result, true
}

// Put stores a successful tool result. Errors are NOT cached.
func (c *ToolCache) Put(toolName, argsJSON, result string) {
	if !isCacheable(toolName) || bypassRequested(argsJSON) {
		return
	}
	if strings.HasPrefix(strings.TrimSpace(strings.ToLower(result)), "error") {
		return
	}
	key := cacheKey(toolName, argsJSON)
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		ent := el.Value.(*cacheEntry)
		ent.result = result
		ent.stored = time.Now()
		c.lru.MoveToFront(el)
		return
	}
	el := c.lru.PushFront(&cacheEntry{key: key, result: result, stored: time.Now()})
	c.items[key] = el
	for c.lru.Len() > cacheMaxEntries {
		back := c.lru.Back()
		if back == nil {
			break
		}
		c.lru.Remove(back)
		delete(c.items, back.Value.(*cacheEntry).key)
	}
}

// sweepExpired walks the LRU from the back evicting entries past their TTL.
// Caller MUST hold c.mu.
func (c *ToolCache) sweepExpired() {
	now := time.Now()
	for el := c.lru.Back(); el != nil; {
		prev := el.Prev()
		ent := el.Value.(*cacheEntry)
		if now.Sub(ent.stored) > cacheTTL {
			c.lru.Remove(el)
			delete(c.items, ent.key)
		} else {
			// LRU order means once we see a non-expired entry walking back,
			// nothing newer than it can be expired either.
			break
		}
		el = prev
	}
}

// Clear drops all entries — called on session reset.
func (c *ToolCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element)
	c.lru.Init()
}

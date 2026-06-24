package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"apexclaw/core"
	_ "apexclaw/tools"
)

func main() {
	core.RegisterBuiltinTools(core.GlobalRegistry)
	fmt.Printf("REPL ready. %d tools loaded.\n", len(core.GlobalRegistry.List()))
	fmt.Println("Type a query (or :q to quit, :reset to clear history, :show to dump history):")

	userID := os.Getenv("OWNER_ID")
	if userID == "" {
		userID = "repl-test"
	}
	fmt.Printf("running as user %q (set OWNER_ID env to override)\n", userID)
	session := core.GetOrCreateAgentSession(userID)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		switch input {
		case ":q", ":quit", ":exit":
			fmt.Println("bye.")
			return
		case ":reset":
			session.Reset()
			fmt.Println("(session reset)")
			continue
		case ":show":
			fmt.Printf("(history has %d messages)\n", session.HistoryLen())
			continue
		}

		busy := session.IsBusy()
		fmt.Printf("(busy=%v)\n", busy)

		startedAt := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		seenChunks := 0
		toolCalls := 0
		toolResults := 0
		commentary := 0
		onChunk := func(chunk string) {
			seenChunks++
			switch {
			case strings.HasPrefix(chunk, "__TOOL_CALL:"):
				toolCalls++
				raw := strings.TrimSuffix(strings.TrimPrefix(chunk, "__TOOL_CALL:"), "__\n")
				id, label, _ := strings.Cut(raw, "\x1f")
				fmt.Printf("  ➜ TOOL_CALL [%s] %s\n", id, label)
			case strings.HasPrefix(chunk, "__TOOL_RESULT:"):
				toolResults++
				raw := strings.TrimSuffix(strings.TrimPrefix(chunk, "__TOOL_RESULT:"), "__\n")
				parts := strings.SplitN(raw, "\x1f", 3)
				if len(parts) == 3 {
					status := parts[2]
					if len(status) > 80 {
						status = status[:80] + "…"
					}
					fmt.Printf("  ✓ TOOL_RESULT [%s] %s → %s\n", parts[0], parts[1], status)
				} else {
					fmt.Printf("  ✓ TOOL_RESULT (unparseable): %q\n", raw)
				}
			case strings.HasPrefix(chunk, "__COMMENTARY:"):
				commentary++
				raw := strings.TrimSuffix(strings.TrimPrefix(chunk, "__COMMENTARY:"), "__\n")
				if len(raw) > 200 {
					raw = raw[:200] + "…"
				}
				fmt.Printf("  💬 COMMENTARY: %s\n", raw)
			default:
				// final reply chunks
				preview := chunk
				if len(preview) > 300 {
					preview = preview[:300] + "…"
				}
				fmt.Printf("  📤 CHUNK: %s\n", strings.ReplaceAll(preview, "\n", "\\n"))
			}
		}
		result, err := session.RunStream(ctx, userID, input, onChunk)
		cancel()

		dur := time.Since(startedAt)
		fmt.Printf("\n--- TURN SUMMARY ---\n")
		fmt.Printf("duration:     %s\n", dur)
		fmt.Printf("chunks:       %d (calls=%d results=%d commentary=%d)\n", seenChunks, toolCalls, toolResults, commentary)
		fmt.Printf("busy@end:     %v\n", session.IsBusy())
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			continue
		}
		fmt.Printf("final reply:\n%s\n", result)
	}
}

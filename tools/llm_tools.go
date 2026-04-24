package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"apexclaw/model"
)

var Humanize = &ToolDef{
	Name:        "humanize",
	Description: "Remove AI writing patterns and make text sound natural and human using intelligent rewriting based on AI detection patterns",
	Args: []ToolArg{
		{Name: "text", Description: "The text to humanize", Required: true},
		{Name: "mode", Description: "Mode: 'aggressive', 'balanced' (default), or 'light'", Required: false},
		{Name: "focus", Description: "Focus area: 'all' (default), 'vocabulary', 'structure', 'tone'", Required: false},
	},
	Execute: func(args map[string]string) string {
		text := args["text"]
		if text == "" {
			return "Error: text is required"
		}

		mode := args["mode"]
		if mode == "" {
			mode = "balanced"
		}

		focus := args["focus"]
		if focus == "" {
			focus = "all"
		}

		return humanizeWithLLM(text, mode, focus)
	},
}

const aiPatternGuide = `Rewrite the text to sound human, not AI-generated. Preserve meaning and facts.

Fix these AI tells:
- Overused words: delve, crucial, pivotal, landscape, testament, showcase, vibrant, intricate, underscore, foster, align, additionally
- Filler: "In order to", "Due to the fact that", "It's important to note"
- Hedging: "could potentially", "arguably", "might have"
- Empty significance: "marks a pivotal moment", "plays a vital role", "reflects broader trends"
- -ing tails: "...highlighting X", "...emphasizing Y"
- Em dashes, curly quotes, emoji decoration, unnecessary bold, title-case headings
- Sycophancy: "I hope this helps", "Let me know", "Here is a..."
- "Not only...but also" / "It's not just...it's" constructions
- Same-length sentences in a row

Vary sentence length. Be specific, not vague. Return ONLY the rewritten text.`

func humanizeWithLLM(text string, mode string, focus string) string {
	client := model.New()

	modeInstructions := map[string]string{
		"aggressive": "Fix ALL AI patterns. Be aggressive in removing AI-isms. Rewrite significantly for maximum naturalness.",
		"balanced":   "Fix moderate AI patterns. Balance between cleaning up obvious patterns while preserving the original intent and structure.",
		"light":      "Fix only the most obvious AI patterns. Minimal rewriting, keep original structure mostly intact.",
	}

	focusInstructions := map[string]string{
		"all":        "Address all types of AI patterns: vocabulary, structure, tone, content.",
		"vocabulary": "Focus mainly on vocabulary: replace AI words, reduce hedging, simplify phrasing.",
		"structure":  "Focus mainly on structure: vary sentence length, fix parallelisms, improve flow.",
		"tone":       "Focus mainly on tone: add personality, remove sycophancy, inject opinions and specificity.",
	}

	modeStr := modeInstructions[mode]
	if modeStr == "" {
		modeStr = modeInstructions["balanced"]
	}

	focusStr := focusInstructions[focus]
	if focusStr == "" {
		focusStr = focusInstructions["all"]
	}

	prompt := fmt.Sprintf(`%s

Mode: %s

Focus: %s

Text to Humanize:
%s`, aiPatternGuide, modeStr, focusStr, text)

	messages := []model.Message{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reply, err := client.Send(ctx, "glm-4.7", messages)
	if err != nil {
		return fmt.Sprintf("Error: humanization failed: %v", err)
	}

	return strings.TrimSpace(reply.Content)
}


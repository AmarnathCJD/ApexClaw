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

const aiPatternGuide = `You are a writing editor specialized in removing AI-generated text patterns to make writing sound natural and human.

## Your Task
Analyze the given text for AI writing patterns and rewrite it to sound more natural and human.

## Signs of AI Writing to Fix

### PERSONALITY AND SOUL
- Every sentence same length/structure → Vary rhythm with short and long sentences
- No opinions, just neutral reporting → Add reactions and perspective
- No acknowledgment of uncertainty/mixed feelings → Show nuance
- No first-person perspective → Use "I" when natural
- No humor, edge, personality → Inject actual voice
- Reads like Wikipedia/press release → Make it conversational

### CONTENT PATTERNS
1. **Undue Emphasis on Significance**: Avoid "marks a pivotal moment", "vital role", "underscores importance", "reflects broader trends"
2. **Undue Notability/Media Coverage**: Don't list citations without context
3. **Superficial -ing Analysis**: Remove "highlighting...", "emphasizing...", "reflecting/symbolizing..."
4. **Promotional Language**: Avoid "breathtaking", "stunning", "nestled", "profound", "renowned", "vibrant", "rich" (figurative)
5. **Vague Attributions**: Replace "Experts argue", "Industry reports", "Observers cite" with specific sources
6. **Formulaic "Challenges" Sections**: Replace with specific facts/dates

### LANGUAGE PATTERNS
7. **Overused AI Vocabulary**: Reduce "Additionally", "align with", "crucial", "delve", "enhance", "fostering", "landscape", "pivotal", "showcase", "testament", "underscore", "vibrant", "interplay", "intricate"
8. **Copula Avoidance**: Replace "serves as", "stands as", "marks", "represents", "boasts", "features" with simple "is/are" or "has"
9. **Negative Parallelisms**: Remove "Not only...but also" and "It's not just...it's" constructions
10. **Rule of Three**: Don't force ideas into groups of three
11. **Elegant Variation**: Don't cycle synonyms for same concept
12. **False Ranges**: Fix "from X to Y" where X and Y aren't on same scale

### STYLE PATTERNS
13. **Em Dash Overuse**: Replace em dashes with commas
14. **Overuse of Boldface**: Remove unnecessary bold formatting
15. **Inline-Header Lists**: Convert to flowing text
16. **Title Case in Headings**: Use normal capitalization unless proper nouns
17. **Emojis**: Remove emoji decoration
18. **Curly Quotes**: Use straight quotes

### COMMUNICATION PATTERNS
19. **Collaborative Artifacts**: Remove "I hope this helps", "Let me know", "here is a"
20. **Knowledge-Cutoff Disclaimers**: Remove hedging about training data
21. **Sycophantic Tone**: Remove excessive positivity and people-pleasing
22. **Filler Phrases**: Replace "In order to" → "To", "Due to the fact that" → "Because", "At this point in time" → "Now"
23. **Excessive Hedging**: Reduce "could potentially", "possibly", "might have", "arguably"
24. **Generic Conclusions**: Replace vague upbeat endings with specific facts

## Your Approach
1. Read the text carefully
2. Identify AI patterns above
3. Rewrite to fix them while preserving meaning
4. Add personality, specificity, and natural rhythm
5. Use concrete details over vague claims
6. Vary sentence structure

## Output
Return ONLY the humanized text without explanations or notes.`

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

var FrontendDesign = &ToolDef{
	Name:        "frontend_design",
	Description: "Create distinctive, production-grade frontend interfaces that avoid generic AI slop aesthetics. Generates working HTML/CSS/JS with exceptional aesthetic direction.",
	Args: []ToolArg{
		{Name: "requirement", Description: "Frontend requirement: component, page, or application to build", Required: true},
		{Name: "context", Description: "Context about purpose, audience, or technical constraints (optional)", Required: false},
		{Name: "aesthetic", Description: "Desired aesthetic direction: 'minimalist', 'maximalist', 'brutalist', 'retro-futuristic', 'organic', 'luxury', 'playful', 'editorial', 'artdeco', 'industrial', or let AI decide", Required: false},
		{Name: "framework", Description: "Framework preference: 'html' (default), 'react', 'vue', 'svelte'", Required: false},
		{Name: "output", Description: "Output format: 'code' (default), 'preview', 'detailed'", Required: false},
	},
	Execute: func(args map[string]string) string {
		requirement := args["requirement"]
		context := args["context"]
		aesthetic := args["aesthetic"]
		framework := args["framework"]
		output := args["output"]

		if requirement == "" {
			return "Error: requirement is required"
		}

		if framework == "" {
			framework = "html"
		}

		if output == "" {
			output = "code"
		}

		return designFrontend(requirement, context, aesthetic, framework, output)
	},
}

const frontendDesignGuide = `You are an exceptional frontend designer and developer creating distinctive, production-grade interfaces that avoid generic AI aesthetics.

## Design Thinking Process
Before coding, understand:
1. **Purpose**: What problem does this interface solve? Who uses it?
2. **Tone**: Pick an extreme aesthetic direction (brutally minimal, maximalist chaos, retro-futuristic, organic, luxury, playful, editorial, brutalist, art deco, industrial, etc.)
3. **Constraints**: Technical requirements (framework, performance, accessibility)
4. **Differentiation**: What's the ONE thing someone will remember about this interface?

CRITICAL: Choose a clear conceptual direction and execute with precision. Bold maximalism AND refined minimalism both work - the key is intentionality, not intensity.

## Frontend Aesthetics Excellence

### Typography
- Choose beautiful, unique, distinctive fonts (NOT generic Arial, Inter, Roboto)
- Pair distinctive display font with refined body font
- Use fonts that feel specifically designed for the context
- Vary fonts across different projects - never repeat the same choices

### Color & Theme
- Commit to cohesive aesthetic using CSS variables
- Dominant colors + sharp accents (timid palettes fail)
- Match color choices to aesthetic direction
- Create visual hierarchy through color, not just size

### Motion & Animation
- CSS-only solutions for HTML (preferred)
- Motion library for React when available
- High-impact orchestrated moments (page load reveals with staggered animation-delay)
- Scroll-triggered animations and surprising hover states
- Prioritize quality over quantity of animations

### Spatial Composition
- Unexpected layouts with asymmetry, overlap, diagonal flow
- Grid-breaking elements and generous negative space
- OR controlled density - never timid ambiguity
- Contextual spacing that breathes

### Backgrounds & Visual Details
- Create atmosphere and depth (not solid colors)
- Creative forms: gradient meshes, noise textures, geometric patterns
- Layered transparencies, dramatic shadows, decorative borders
- Custom cursors, grain overlays, contextual effects
- Details that match the overall aesthetic

## NEVER Do This
- Generic AI aesthetics: Purple gradients on white backgrounds
- Overused fonts: Space Grotesk, system fonts, common defaults
- Predictable layouts or component patterns
- Cookie-cutter design lacking context-specific character
- Same design repeated across projects

## Implementation Approach
- Match complexity to vision: maximalist needs elaborate code/animations, minimalist needs restraint/precision
- Elegance = executing the vision well
- Production-grade: fully functional, accessible, performant
- Memorable: one distinctive detail people remember

## Output Format
Return ONLY production-ready code without explanations. Include:
- Complete, working implementation
- Embedded CSS and JS (no external deps unless necessary)
- Detailed aesthetic comments explaining design choices
- Accessibility considerations
- Performance optimizations`

func designFrontend(requirement string, ctx string, aesthetic string, framework string, output string) string {
	client := model.New()

	aestheticDir := aesthetic
	if aestheticDir == "" {
		aestheticDir = "AI-determined based on context"
	}

	prompt := fmt.Sprintf(`%s

## Your Task
Create a distinctive, production-grade frontend for this requirement:

**Requirement**: %s

**Additional Context**: %s

**Framework**: %s

**Aesthetic Direction**: %s

**Output Format**: %s

Design thinking first:
1. Understand the purpose and audience
2. Commit to a bold, intentional aesthetic direction
3. Make unexpected creative choices
4. Execute with precision and attention to detail
5. Ensure it's truly memorable and distinctive

Then implement working, production-ready code that is:
- Fully functional and tested
- Visually striking with cohesive aesthetic
- Meticulously refined in every detail
- Accessible and performant
- Completely unique (not generic AI slop)

Remember: You're capable of extraordinary creative work. Don't hold back. Show what can truly be created when thinking outside the box.`,
		frontendDesignGuide, requirement, ctx, framework, aestheticDir, output)

	messages := []model.Message{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	cntx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	reply, err := client.Send(cntx, "claude-opus-4-6", messages)
	if err != nil {
		return fmt.Sprintf("Error: frontend design failed: %v", err)
	}

	return strings.TrimSpace(reply.Content)
}

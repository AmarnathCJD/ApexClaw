package tools

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
)

var Calculate = &ToolDef{
	Name:        "calculate",
	Description: "Evaluate a math expression (supports +, -, *, /, %, sqrt, pow, abs, floor, ceil, round, pi, e)",
	Args: []ToolArg{
		{Name: "expr", Description: "Expression to evaluate, e.g. '2 * pi * 5' or 'sqrt(144)'", Required: true},
	},
	Execute: func(args map[string]string) string {
		expr := strings.TrimSpace(args["expr"])
		if expr == "" {
			return "Error: expr is required"
		}
		result, err := evalMath(expr)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		if result == math.Trunc(result) {
			return fmt.Sprintf("%g", result)
		}
		return strconv.FormatFloat(result, 'f', 10, 64)
	},
}

var Random = &ToolDef{
	Name:        "random",
	Description: "Generate a random integer between min and max (inclusive)",
	Args: []ToolArg{
		{Name: "min", Description: "Minimum value (default 1)", Required: false},
		{Name: "max", Description: "Maximum value (default 100)", Required: false},
	},
	Execute: func(args map[string]string) string {
		min := 1
		max := 100
		if v := args["min"]; v != "" {
			fmt.Sscanf(v, "%d", &min)
		}
		if v := args["max"]; v != "" {
			fmt.Sscanf(v, "%d", &max)
		}
		if min > max {
			min, max = max, min
		}
		n := rand.Intn(max-min+1) + min
		return fmt.Sprintf("%d", n)
	},
}

func evalMath(expr string) (float64, error) {
	expr = strings.ReplaceAll(expr, "pi", strconv.FormatFloat(math.Pi, 'f', -1, 64))
	expr = strings.ReplaceAll(expr, " e ", " "+strconv.FormatFloat(math.E, 'f', -1, 64)+" ")

	singleFns := []struct {
		name string
		fn   func(float64) float64
	}{
		{"sqrt", math.Sqrt},
		{"abs", math.Abs},
		{"floor", math.Floor},
		{"ceil", math.Ceil},
		{"round", math.Round},
	}
	for _, fp := range singleFns {
		prefix := fp.name + "("
		for {
			idx := strings.Index(expr, prefix)
			if idx == -1 {
				break
			}
			end := strings.Index(expr[idx:], ")")
			if end == -1 {
				break
			}
			inner := expr[idx+len(prefix) : idx+end]
			val, err := evalMath(strings.TrimSpace(inner))
			if err != nil {
				return 0, err
			}
			expr = expr[:idx] + strconv.FormatFloat(fp.fn(val), 'f', -1, 64) + expr[idx+end+1:]
		}
	}

	for {
		idx := strings.Index(expr, "pow(")
		if idx == -1 {
			break
		}
		end := strings.Index(expr[idx:], ")")
		if end == -1 {
			break
		}
		inner := expr[idx+4 : idx+end]
		parts := strings.SplitN(inner, ",", 2)
		if len(parts) != 2 {
			break
		}
		base, e1 := evalMath(strings.TrimSpace(parts[0]))
		exp, e2 := evalMath(strings.TrimSpace(parts[1]))
		if e1 != nil || e2 != nil {
			return 0, fmt.Errorf("invalid pow arguments")
		}
		expr = expr[:idx] + strconv.FormatFloat(math.Pow(base, exp), 'f', -1, 64) + expr[idx+end+1:]
	}

	tokens := tokenize(expr)
	return parseExpr(tokens)
}

func tokenize(expr string) []string {
	var tokens []string
	var cur strings.Builder
	for _, ch := range expr {
		switch ch {
		case '+', '-', '*', '/', '%', '(', ')':
			if cur.Len() > 0 {
				tokens = append(tokens, strings.TrimSpace(cur.String()))
				cur.Reset()
			}
			tokens = append(tokens, string(ch))
		case ' ', '\t':
			if cur.Len() > 0 {
				tokens = append(tokens, strings.TrimSpace(cur.String()))
				cur.Reset()
			}
		default:
			cur.WriteRune(ch)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, strings.TrimSpace(cur.String()))
	}
	return tokens
}

type parser struct {
	tokens []string
	pos    int
}

func parseExpr(tokens []string) (float64, error) {
	p := &parser{tokens: tokens}
	return p.addSub()
}

func (p *parser) peek() string {
	if p.pos >= len(p.tokens) {
		return ""
	}
	return p.tokens[p.pos]
}

func (p *parser) consume() string {
	t := p.peek()
	p.pos++
	return t
}

func (p *parser) addSub() (float64, error) {
	left, err := p.mulDiv()
	if err != nil {
		return 0, err
	}
	for p.peek() == "+" || p.peek() == "-" {
		op := p.consume()
		right, err := p.mulDiv()
		if err != nil {
			return 0, err
		}
		if op == "+" {
			left += right
		} else {
			left -= right
		}
	}
	return left, nil
}

func (p *parser) mulDiv() (float64, error) {
	left, err := p.unary()
	if err != nil {
		return 0, err
	}
	for p.peek() == "*" || p.peek() == "/" || p.peek() == "%" {
		op := p.consume()
		right, err := p.unary()
		if err != nil {
			return 0, err
		}
		switch op {
		case "*":
			left *= right
		case "/":
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			left /= right
		case "%":
			left = math.Mod(left, right)
		}
	}
	return left, nil
}

func (p *parser) unary() (float64, error) {
	if p.peek() == "-" {
		p.consume()
		v, err := p.primary()
		return -v, err
	}
	if p.peek() == "+" {
		p.consume()
	}
	return p.primary()
}

func (p *parser) primary() (float64, error) {
	tok := p.peek()
	if tok == "(" {
		p.consume()
		v, err := p.addSub()
		if err != nil {
			return 0, err
		}
		if p.peek() == ")" {
			p.consume()
		}
		return v, nil
	}
	p.consume()
	v, err := strconv.ParseFloat(tok, 64)
	if err != nil {
		return 0, fmt.Errorf("unexpected token %q", tok)
	}
	return v, nil
}

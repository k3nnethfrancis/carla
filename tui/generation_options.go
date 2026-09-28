package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// The tab defines what the positional count produces. Tokens are always a cap.
type generationOptions struct {
	Count, Tokens, Turns, Loops int
	Message, Evaluation         string
}

func (o generationOptions) apply(args map[string]any) {
	if o.Evaluation != "" {
		args["eval"] = o.Evaluation
	}
	if o.Message != "" {
		args["message"] = o.Message
	}
	if o.Loops > 0 {
		args["loops"] = o.Loops
	}
	if o.Count > 0 {
		args["count"] = o.Count
	}
	if o.Tokens != 0 {
		args["n_predict"] = o.Tokens
	}
	if o.Turns > 0 {
		args["turns"] = o.Turns
	}
}
func parseGenerationOptions(input, id string) (generationOptions, error) {
	var out generationOptions
	fields, err := generationWords(input)
	if err != nil {
		return out, err
	}
	seen := map[string]bool{}
	fail := func() (generationOptions, error) {
		return out, fmt.Errorf("use /loom 5 --tokens 1024 --loops 4 (Simulator also accepts --turns N and --msg with quoted text); token ranges are not supported")
	}
	for i := 1; i < len(fields); i++ {
		key, value, inline := strings.Cut(fields[i], "=")
		if id != "loom" && id != "continue" && id != "generate" {
			return fail()
		}
		positional := !strings.HasPrefix(key, "-") && !inline
		if positional {
			value = key
			key = "--tokens"
			if id == "loom" {
				key = "--count"
			}
		}
		if key != "--tokens" && (id != "loom" || (key != "--count" && key != "-n" && key != "--turns" && key != "--loops" && key != "--msg" && key != "--message" && key != "--eval")) {
			return fail()
		}
		if key == "--msg" {
			key = "--message"
		}
		if key == "-n" {
			key = "--count"
		}
		if seen[key] {
			return fail()
		}
		seen[key] = true
		if !inline && !positional {
			i++
			if i >= len(fields) {
				return fail()
			}
			value = fields[i]
		}
		if key == "--eval" {
			if strings.TrimSpace(value) == "" {
				return fail()
			}
			out.Evaluation = value
			continue
		}
		if key == "--message" {
			if strings.TrimSpace(value) == "" {
				return out, fmt.Errorf("--message needs a nonempty quoted message")
			}
			out.Message = value
			continue
		}
		n, err := strconv.Atoi(value)
		if key == "--tokens" && strings.EqualFold(value, "max") {
			n, err = -1, nil
		}
		if err != nil || (n < 1 && !(key == "--tokens" && n == -1 && strings.EqualFold(value, "max"))) {
			return fail()
		}
		switch key {
		case "--tokens":
			out.Tokens = n
		case "--loops":
			out.Loops = n
		case "--turns":
			out.Turns = n
		default:
			if out.Count != 0 {
				return fail()
			}
			out.Count = n
		}
	}
	return out, nil
}

// Parse quoted text without a shell: no expansion or execution, and no splitting
// spaces inside the visitor message. Numeric options retain their existing syntax.
func generationWords(input string) ([]string, error) {
	var words []string
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	for _, r := range input {
		if escaped {
			word.WriteRune(r)
			escaped = false
			started = true
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
			continue
		}
		if unicode.IsSpace(r) {
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
			continue
		}
		word.WriteRune(r)
		started = true
	}
	if quote != 0 || escaped {
		return nil, fmt.Errorf("close the message quote or escape before running /loom")
	}
	if started {
		words = append(words, word.String())
	}
	return words, nil
}

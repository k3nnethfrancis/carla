package main

import (
	"fmt"
	"strconv"
	"strings"
)

// The tab defines what the positional count produces. Tokens are always a cap.
type generationOptions struct{ Count, Tokens, Turns, Loops int }

func (o generationOptions) apply(args map[string]any) {
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
	fields := strings.Fields(input)
	seen := map[string]bool{}
	fail := func() (generationOptions, error) {
		return out, fmt.Errorf("use /loom 5 --tokens 1024 --loops 4 (Simulator also accepts --turns N); token ranges are not supported")
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
		if key != "--tokens" && (id != "loom" || (key != "--count" && key != "-n" && key != "--turns" && key != "--loops")) {
			return fail()
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

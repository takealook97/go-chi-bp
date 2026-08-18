package httpapi

import (
	"errors"
	"fmt"
	"strings"
)

// The contract test needs the operations declared in api/openapi.yaml, and the
// repository has no YAML dependency to read them with. The reader below is
// therefore deliberately narrow: it understands the block-mapping subset the
// contract is written in and refuses anything else, so an unsupported document
// fails with an explanation instead of silently yielding a partial set.

var openAPIMethods = map[string]struct{}{
	"delete":  {},
	"get":     {},
	"head":    {},
	"options": {},
	"patch":   {},
	"post":    {},
	"put":     {},
	"trace":   {},
}

// errNoPathsSection reports a document without a usable paths block mapping.
var errNoPathsSection = errors.New("no paths block mapping found")

// openAPIOperations returns the "METHOD /path" set declared under paths.
//
// Indentation is derived from the document rather than assumed, so reindenting
// the contract cannot quietly change what this reads. Block scalars are
// skipped, because a description may legally contain lines that look like
// paths or operations.
func openAPIOperations(contents string) (map[string]struct{}, error) {
	lines := strings.Split(contents, "\n")

	start, pathsIndent, err := findPathsSection(lines)
	if err != nil {
		return nil, err
	}

	operations := make(map[string]struct{})
	currentPath := ""
	pathIndent, operationIndent := -1, -1
	blockScalarIndent := -1

	for _, line := range lines[start:] {
		if isBlank(line) || isComment(line) {
			continue
		}

		indent := leadingSpaces(line)
		if blockScalarIndent >= 0 {
			if indent > blockScalarIndent {
				continue
			}
			blockScalarIndent = -1
		}
		if indent <= pathsIndent {
			break
		}

		key, value, ok := splitKey(line)
		if !ok {
			continue
		}
		if isBlockScalar(value) {
			blockScalarIndent = indent

			continue
		}

		switch {
		case pathIndent < 0 || indent == pathIndent:
			if !strings.HasPrefix(key, "/") {
				return nil, fmt.Errorf("unexpected key %q where a path was expected", key)
			}
			pathIndent, currentPath = indent, key
		case indent > pathIndent && currentPath != "":
			if operationIndent < 0 {
				operationIndent = indent
			}
			if indent != operationIndent {
				continue
			}
			if _, isMethod := openAPIMethods[key]; isMethod {
				operations[strings.ToUpper(key)+" "+normalizeRoute(currentPath)] = struct{}{}
			}
		}
	}

	if len(operations) == 0 {
		return nil, errors.New("paths section declared no operations")
	}

	return operations, nil
}

// findPathsSection returns the first line index inside paths and its indent.
func findPathsSection(lines []string) (int, int, error) {
	for index, line := range lines {
		if isBlank(line) || isComment(line) {
			continue
		}

		key, value, ok := splitKey(line)
		if !ok || key != "paths" {
			continue
		}
		if value != "" {
			return 0, 0, fmt.Errorf("paths is not a block mapping: %q", value)
		}

		return index + 1, leadingSpaces(line), nil
	}

	return 0, 0, errNoPathsSection
}

// splitKey separates "key: value", reporting whether the line declares a key.
// Quotes around the key are removed so that "429": reads as 429.
func splitKey(line string) (string, string, bool) {
	key, value, found := strings.Cut(strings.TrimSpace(line), ":")
	if !found || strings.HasPrefix(key, "-") {
		return "", "", false
	}

	key = strings.Trim(strings.TrimSpace(key), `"'`)
	if key == "" {
		return "", "", false
	}

	return key, strings.TrimSpace(stripComment(value)), true
}

// isBlockScalar reports whether a value opens a literal or folded scalar.
func isBlockScalar(value string) bool {
	return strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">")
}

// stripComment removes a trailing comment from an unquoted value.
func stripComment(value string) string {
	if strings.ContainsAny(value, `"'`) {
		return value
	}
	if index := strings.Index(value, "#"); index >= 0 {
		return value[:index]
	}

	return value
}

func isBlank(line string) bool {
	return strings.TrimSpace(line) == ""
}

func isComment(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "#")
}

func leadingSpaces(value string) int {
	return len(value) - len(strings.TrimLeft(value, " "))
}

func normalizeRoute(route string) string {
	if route != "/" {
		return strings.TrimSuffix(route, "/")
	}

	return route
}

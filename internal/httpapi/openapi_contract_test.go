package httpapi

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestRouterMatchesOpenAPIOperations(t *testing.T) {
	t.Parallel()

	routes, ok := testRouter(func(_ context.Context) error { return nil }).(chi.Routes)
	if !ok {
		t.Fatal("router does not expose Chi routes")
	}

	actual := make(map[string]struct{})
	if err := chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if _, documented := openAPIMethods[strings.ToLower(method)]; documented {
			actual[method+" "+normalizeRoute(route)] = struct{}{}
		}

		return nil
	}); err != nil {
		t.Fatalf("walk router: %v", err)
	}

	expected := readOpenAPIOperations(t)
	if missing, extra := operationDifference(expected, actual), operationDifference(actual, expected); len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("router and OpenAPI operations differ\nmissing from router: %v\nmissing from OpenAPI: %v", missing, extra)
	}
}

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

func readOpenAPIOperations(t *testing.T) map[string]struct{} {
	t.Helper()

	contents, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI contract: %v", err)
	}

	operations := make(map[string]struct{})
	currentPath := ""
	inPaths := false
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "paths:" {
			inPaths = true

			continue
		}
		if !inPaths {
			continue
		}
		if line != "" && line[0] != ' ' {
			break
		}
		if strings.HasPrefix(line, "  /") && strings.HasSuffix(line, ":") {
			currentPath = strings.TrimSuffix(strings.TrimSpace(line), ":")

			continue
		}
		if currentPath == "" || leadingSpaces(line) != 4 {
			continue
		}

		method := strings.TrimSuffix(strings.TrimSpace(line), ":")
		if _, ok := openAPIMethods[method]; ok {
			operations[strings.ToUpper(method)+" "+normalizeRoute(currentPath)] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan OpenAPI contract: %v", err)
	}

	return operations
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

func operationDifference(left, right map[string]struct{}) []string {
	result := make([]string, 0)
	for operation := range left {
		if _, ok := right[operation]; !ok {
			result = append(result, operation)
		}
	}
	sort.Strings(result)

	return result
}

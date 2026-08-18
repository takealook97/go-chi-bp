package httpapi

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
)

// This test proves that every documented operation is routed and that no route
// is undocumented. It does not verify schemas; TestResponsesSatisfyTheOpenAPISchemas
// does that.
func TestRouterMatchesOpenAPIOperations(t *testing.T) {
	t.Parallel()

	routes, ok := testRouter(func(_ context.Context) error { return nil }).(chi.Routes)
	if !ok {
		t.Fatal("router does not expose Chi routes")
	}

	actual := make(map[string]struct{})
	if err := chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		actual[strings.ToUpper(method)+" "+normalizeRoute(route)] = struct{}{}

		return nil
	}); err != nil {
		t.Fatalf("walk router: %v", err)
	}

	expected := openAPIOperations(t)
	if missing, extra := operationDifference(expected, actual), operationDifference(actual, expected); len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("router and OpenAPI operations differ\nmissing from router: %v\nmissing from OpenAPI: %v", missing, extra)
	}
}

// openAPIOperations returns the "METHOD /path" set the contract declares.
func openAPIOperations(t *testing.T) map[string]struct{} {
	t.Helper()

	operations := make(map[string]struct{})
	for path, item := range loadContract(t).Paths.Map() {
		for method := range item.Operations() {
			operations[method+" "+normalizeRoute(path)] = struct{}{}
		}
	}
	if len(operations) == 0 {
		t.Fatal("the contract declares no operations")
	}

	return operations
}

// loadContract reads and validates api/openapi.yaml.
func loadContract(t *testing.T) *openapi3.T {
	t.Helper()

	document, err := (&openapi3.Loader{}).LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("load OpenAPI contract: %v", err)
	}
	if err := document.Validate(t.Context()); err != nil {
		t.Fatalf("validate OpenAPI contract: %v", err)
	}

	return document
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

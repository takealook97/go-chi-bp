package httpapi

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// This test proves that every documented operation is routed and that no route
// is undocumented. It does not verify schemas, so a response may still violate
// the contract it is matched against; those cases belong in handler tests.
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

	expected, err := openAPIOperations(readContract(t))
	if err != nil {
		t.Fatalf("read OpenAPI operations: %v", err)
	}

	if missing, extra := operationDifference(expected, actual), operationDifference(actual, expected); len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("router and OpenAPI operations differ\nmissing from router: %v\nmissing from OpenAPI: %v", missing, extra)
	}
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

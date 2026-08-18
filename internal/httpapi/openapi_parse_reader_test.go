package httpapi

import (
	"errors"
	"os"
	"sort"
	"strings"
	"testing"
)

func TestOpenAPIOperationsReadsTheContract(t *testing.T) {
	t.Parallel()

	operations, err := openAPIOperations(readContract(t))
	if err != nil {
		t.Fatalf("openAPIOperations() unexpected error: %v", err)
	}

	want := []string{
		"DELETE /v1/widgets/{widgetID}",
		"GET /health/live",
		"GET /health/ready",
		"GET /v1/widgets",
		"GET /v1/widgets/{widgetID}",
		"POST /v1/widgets",
	}
	got := sortedKeys(operations)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("operations = %v, want %v", got, want)
	}
}

func TestOpenAPIOperationsSkipsBlockScalarText(t *testing.T) {
	t.Parallel()

	// A description is free text. Its lines may look like structure, and
	// reading them would invent routes the router cannot serve.
	document := `paths:
  /widgets:
    get:
      description: |
        These lines are prose, not structure:
          post:
          /nested:
      summary: list
`

	operations, err := openAPIOperations(document)
	if err != nil {
		t.Fatalf("openAPIOperations() unexpected error: %v", err)
	}
	if got := sortedKeys(operations); strings.Join(got, ",") != "GET /widgets" {
		t.Fatalf("operations = %v, want only GET /widgets", got)
	}
}

func TestOpenAPIOperationsIgnoresComments(t *testing.T) {
	t.Parallel()

	document := `paths:
  # /commented:
  #   get:
  /widgets:
    get:
      summary: list
`

	operations, err := openAPIOperations(document)
	if err != nil {
		t.Fatalf("openAPIOperations() unexpected error: %v", err)
	}
	if got := sortedKeys(operations); strings.Join(got, ",") != "GET /widgets" {
		t.Fatalf("operations = %v, want only GET /widgets", got)
	}
}

func TestOpenAPIOperationsFollowsDocumentIndentation(t *testing.T) {
	t.Parallel()

	document := `paths:
    /widgets:
        get:
            summary: list
        post:
            summary: create
`

	operations, err := openAPIOperations(document)
	if err != nil {
		t.Fatalf("openAPIOperations() unexpected error: %v", err)
	}
	if got := sortedKeys(operations); strings.Join(got, ",") != "GET /widgets,POST /widgets" {
		t.Fatalf("operations = %v, want both widget operations", got)
	}
}

func TestOpenAPIOperationsIgnoresNestedMethodNames(t *testing.T) {
	t.Parallel()

	// "post" is a plausible schema property name. Only the operation level counts.
	document := `paths:
  /widgets:
    get:
      responses:
        "200":
          schema:
            properties:
              post:
                type: string
`

	operations, err := openAPIOperations(document)
	if err != nil {
		t.Fatalf("openAPIOperations() unexpected error: %v", err)
	}
	if got := sortedKeys(operations); strings.Join(got, ",") != "GET /widgets" {
		t.Fatalf("operations = %v, want only GET /widgets", got)
	}
}

func TestOpenAPIOperationsRejectsUnsupportedDocuments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		document string
		wantErr  string
	}{
		{name: "no paths", document: "openapi: 3.1.0\n", wantErr: errNoPathsSection.Error()},
		{name: "flow mapping", document: "paths: {}\n", wantErr: "not a block mapping"},
		{name: "no operations", document: "paths:\n  /widgets:\n    parameters: []\n", wantErr: "declared no operations"},
		{name: "unexpected key", document: "paths:\n  widgets:\n    get:\n      summary: x\n", wantErr: "where a path was expected"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := openAPIOperations(test.document)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("openAPIOperations() error = %v, want one containing %q", err, test.wantErr)
			}
		})
	}
}

func TestOpenAPIOperationsReportsAMissingPathsSectionAsASentinel(t *testing.T) {
	t.Parallel()

	_, err := openAPIOperations("openapi: 3.1.0\n")
	if !errors.Is(err, errNoPathsSection) {
		t.Fatalf("error = %v, want errNoPathsSection", err)
	}
}

func readContract(t *testing.T) string {
	t.Helper()

	contents, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI contract: %v", err)
	}

	return string(contents)
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}

package widget

import (
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// The name length and page size bounds are restated on purpose: the OpenAPI
// contract publishes them to clients, this package enforces them for every
// caller, and the widgets table's CHECK constraints enforce them for every
// writer. Three independent rejections of the same value is the intent; three
// values drifting apart is not, and nothing else in the build compares them.
//
// Adopters renaming the example capability will change these numbers first, so
// the guard matters most in exactly the repository this template becomes.

const contractPath = "../../api/openapi.yaml"

// migrationNameLengthPattern matches the CHECK constraint that bounds the stored
// name, in whichever migration defines or later redefines it.
var migrationNameLengthPattern = regexp.MustCompile(`char_length\(name\) <= (\d+)`)

func TestNameLengthBoundMatchesTheContractAndTheDatabase(t *testing.T) {
	t.Parallel()

	schema := createWidgetNameSchema(t)
	if schema.MaxLength == nil {
		t.Fatal("the contract publishes no maximum name length")
	}
	if got := *schema.MaxLength; got != maximumNameLength {
		t.Errorf("contract maxLength = %d, want %d", got, maximumNameLength)
	}
	if got := schema.MinLength; got != 1 {
		t.Errorf("contract minLength = %d, want 1 to match the service's empty-name rejection", got)
	}

	bounds := migrationNameLengthBounds(t)
	if len(bounds) == 0 {
		t.Fatal("no migration bounds the widget name length")
	}
	for _, bound := range bounds {
		if bound != maximumNameLength {
			t.Errorf("migration CHECK bound = %d, want %d", bound, maximumNameLength)
		}
	}
}

func TestListLimitBoundsMatchTheContract(t *testing.T) {
	t.Parallel()

	schema := listLimitSchema(t)
	if schema.Max == nil {
		t.Fatal("the contract publishes no maximum page size")
	}
	if got := int(*schema.Max); got != maximumListLimit {
		t.Errorf("contract maximum = %d, want %d", got, maximumListLimit)
	}
	if schema.Default == nil {
		t.Fatal("the contract publishes no default page size")
	}
	if got := numberValue(t, schema.Default); got != DefaultListLimit {
		t.Errorf("contract default = %d, want %d", got, DefaultListLimit)
	}
}

// createWidgetNameSchema returns the published schema of the created widget name.
func createWidgetNameSchema(t *testing.T) *openapi3.Schema {
	t.Helper()

	operation := contractPathItem(t).Post
	if operation == nil || operation.RequestBody == nil || operation.RequestBody.Value == nil {
		t.Fatal("the contract declares no createWidget request body")
	}
	media := operation.RequestBody.Value.Content.Get("application/json")
	if media == nil || media.Schema == nil || media.Schema.Value == nil {
		t.Fatal("the createWidget request body declares no JSON schema")
	}
	property, ok := media.Schema.Value.Properties["name"]
	if !ok || property.Value == nil {
		t.Fatal("the createWidget request body declares no name property")
	}

	return property.Value
}

// listLimitSchema returns the published schema of the limit query parameter.
func listLimitSchema(t *testing.T) *openapi3.Schema {
	t.Helper()

	operation := contractPathItem(t).Get
	if operation == nil {
		t.Fatal("the contract declares no listWidgets operation")
	}
	parameter := operation.Parameters.GetByInAndName("query", "limit")
	if parameter == nil || parameter.Schema == nil || parameter.Schema.Value == nil {
		t.Fatal("the listWidgets operation declares no limit parameter schema")
	}

	return parameter.Schema.Value
}

func contractPathItem(t *testing.T) *openapi3.PathItem {
	t.Helper()

	document, err := (&openapi3.Loader{}).LoadFromFile(contractPath)
	if err != nil {
		t.Fatalf("load OpenAPI contract: %v", err)
	}
	item := document.Paths.Find("/v1/widgets")
	if item == nil {
		t.Fatal("the contract declares no /v1/widgets path")
	}

	return item
}

// migrationNameLengthBounds returns every name length bound the migrations set.
func migrationNameLengthBounds(t *testing.T) []int {
	t.Helper()

	// Reading through a directory filesystem keeps the migration names out of
	// any path the process could be talked into opening.
	migrations := os.DirFS("../../db/migrations")
	paths, err := fs.Glob(migrations, "*.sql")
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}

	bounds := make([]int, 0, len(paths))
	for _, path := range paths {
		content, readErr := fs.ReadFile(migrations, path)
		if readErr != nil {
			t.Fatalf("read migration %s: %v", path, readErr)
		}
		for _, match := range migrationNameLengthPattern.FindAllStringSubmatch(string(content), -1) {
			bounds = append(bounds, parseBound(t, match[1]))
		}
	}

	return bounds
}

func parseBound(t *testing.T, value string) int {
	t.Helper()

	bound, err := strconv.Atoi(value)
	if err != nil {
		t.Fatalf("parse bound %q: %v", value, err)
	}

	return bound
}

// numberValue reads a contract number, which YAML decoding may present as either
// a float or an integer depending on how the value was written.
func numberValue(t *testing.T, value any) int {
	t.Helper()

	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	default:
		t.Fatalf("contract value %v has unsupported type %T", value, value)

		return 0
	}
}

package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The depguard rules in .golangci.yml are what keep a capability's business code
// from importing the router, the driver, generated queries, or configuration,
// and what keep its adapters from reaching around the application service. This
// runs them against capabilities built for the purpose and fails when a ban does
// not fire.
//
// Two kinds of ban are checked separately because they fail differently. Most
// match the layout, so a capability placed under internal/ inherits them and a
// module added tomorrow is covered without touching the configuration. The two
// naming a capability's own PostgreSQL adapter cannot: depguard matches a banned
// package by prefix, so they are restated per capability, and a capability that
// gains an adapter without them lints green while that boundary goes unenforced.
//
// Reading the configuration instead would only prove that ban text exists. Three
// edits leave every ban byte-for-byte in place and stop all enforcement: removing
// `depguard` from the enabled linters, adding one negation to a rule's `files:`
// selector, or renaming the module in go.mod without updating the ban prefixes,
// which leaves them matching an import path nothing uses. A guard that passes
// through all three is worse than no guard, because the repository documents it
// as the reason a missing ban cannot go unnoticed.

const repositoryRoot = ".."

// bannedImport is one import a rule must reject, and the file that makes it.
type bannedImport struct {
	rule    string
	file    string
	pkg     string
	imports string
}

// These tests are not parallel: golangci-lint takes an exclusive lock and
// refuses to run while another instance is running, so two of them racing fail
// with an error about a parallel run rather than with a finding.
func TestDepguardRejectsEveryBannedImport(t *testing.T) {
	linter := pinnedLinter(t)
	module := modulePath(t)

	for _, capability := range capabilities(t) {
		t.Run(capability, func(t *testing.T) {
			reported := runDepguard(t, linter, capabilityFixture(t, module, capability))
			for _, banned := range append(layoutBans(module, capability), adapterBans(module, capability)...) {
				assertRejected(t, reported, banned)
			}
		})
	}
}

// A capability added tomorrow inherits the layout-matched bans with no edit to
// the configuration. That is the claim the rules make by matching on layout
// rather than on a name, and it is the one an adopter relies on.
func TestDepguardCoversACapabilityThatDoesNotExistYet(t *testing.T) {
	linter := pinnedLinter(t)
	module := modulePath(t)
	capability := "unlisted"

	reported := runDepguard(t, linter, capabilityFixture(t, module, capability))
	for _, banned := range layoutBans(module, capability) {
		assertRejected(t, reported, banned)
	}
}

// layoutBans are the bans every capability inherits from the file selectors.
func layoutBans(module string, capability string) []bannedImport {
	return []bannedImport{
		{"business-layer", "internal/" + capability + "/service.go", "github.com/go-chi/chi/v5", "the router"},
		{"business-layer", "internal/" + capability + "/service.go", "github.com/jackc/pgx/v5", "the driver"},
		{"business-layer", "internal/" + capability + "/service.go", module + "/internal/platform/config", "configuration"},
		{"http-adapter", "internal/" + capability + "/" + capability + "http/handler.go", "github.com/jackc/pgx/v5", "the driver"},
		{"postgres-adapter", "internal/" + capability + "/" + capability + "postgres/repository.go", "github.com/go-chi/chi/v5", "the router"},
		{"postgres-adapter", "internal/" + capability + "/" + capability + "postgres/repository.go", module + "/internal/httpapi", "HTTP contracts"},
	}
}

// adapterBans name one capability's own PostgreSQL adapter, so each capability
// restates them.
func adapterBans(module string, capability string) []bannedImport {
	adapter := module + "/internal/" + capability + "/" + capability + "postgres"

	return []bannedImport{
		{"business-layer", "internal/" + capability + "/service.go", adapter + "/dbgen", "generated queries"},
		{"http-adapter", "internal/" + capability + "/" + capability + "http/handler.go", adapter, "the persistence adapter"},
	}
}

func assertRejected(t *testing.T, reported map[string]bool, banned bannedImport) {
	t.Helper()

	if !reported[banned.file+" "+banned.pkg] {
		t.Errorf("%s may import %s from %s: depguard reported nothing, so %s is not kept out of it",
			banned.rule, banned.pkg, banned.file, banned.imports)
	}
}

// capabilities returns the capability directories under internal/, recognized by
// the adapter packages named after them.
func capabilities(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(repositoryRoot, "..", "internal"))
	if err != nil {
		t.Fatalf("read internal packages: %v", err)
	}

	found := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		for _, suffix := range []string{"http", "postgres"} {
			if info, err := os.Stat(filepath.Join(repositoryRoot, "..", "internal", name, name+suffix)); err == nil && info.IsDir() {
				found = append(found, name)

				break
			}
		}
	}
	if len(found) == 0 {
		t.Fatal("no capability owns an adapter, so this guard would pass without checking anything")
	}

	return found
}

// pinnedLinter returns the repository's own golangci-lint, or skips. The binary
// is built by make check, so a skip here means the tools were not installed
// rather than that the boundaries hold.
func pinnedLinter(t *testing.T) string {
	t.Helper()

	linter := filepath.Join(repositoryRoot, "..", "bin", runtime.GOOS+"-"+runtime.GOARCH, "golangci-lint")
	if _, err := os.Stat(linter); err != nil {
		t.Skipf("the pinned linter is not installed: run make check-tools")
	}
	absolute, err := filepath.Abs(linter)
	if err != nil {
		t.Fatalf("resolve linter path: %v", err)
	}

	return absolute
}

// capabilityFixture writes a module that reuses this repository's lint
// configuration and dependency versions, holding one capability whose files
// import exactly what the rules forbid. It is built outside the repository so
// that the violations it needs cannot be linted as production code.
func capabilityFixture(t *testing.T, module string, capability string) string {
	t.Helper()

	directory := t.TempDir()

	// Reading and writing through roots keeps both ends of the copy inside the
	// directory they belong to, whatever the names are.
	repository, err := os.OpenRoot(filepath.Join(repositoryRoot, ".."))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer func() { _ = repository.Close() }()

	fixture, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatalf("open fixture directory: %v", err)
	}
	defer func() { _ = fixture.Close() }()

	for _, name := range []string{"go.mod", "go.sum", ".golangci.yml"} {
		content, readErr := fs.ReadFile(repository.FS(), name)
		if readErr != nil {
			t.Fatalf("read %s: %v", name, readErr)
		}
		if writeErr := fixture.WriteFile(name, content, 0o600); writeErr != nil {
			t.Fatalf("write %s: %v", name, writeErr)
		}
	}

	// The banned packages must exist for the linter to load the fixture, but
	// only their import paths matter, so each is an empty package.
	adapter := "internal/" + capability + "/" + capability + "postgres"
	// The files reference what they import rather than importing it blankly.
	// revive reports a blank import on the same line depguard would, and
	// golangci-lint keeps one issue per line, so a blank import would hide the
	// finding this test exists to see.
	files := map[string]string{
		"internal/platform/config/config.go": "// Package config is a fixture.\npackage config\n\n// Fixture is referenced by the capability.\ntype Fixture struct{}\n",
		"internal/httpapi/httpapi.go":        "// Package httpapi is a fixture.\npackage httpapi\n\n// Fixture is referenced by the adapter.\ntype Fixture struct{}\n",
		adapter + "/dbgen/queries.go":        "// Package dbgen is a fixture.\npackage dbgen\n\n// Fixture is referenced by the capability.\ntype Fixture struct{}\n",
		adapter + "/repository.go": "// Package " + capability + "postgres is a fixture.\npackage " + capability + "postgres\n\nimport (\n\t\"github.com/go-chi/chi/v5\"\n\n\t\"" +
			module + "/internal/httpapi\"\n)\n\n// Fixture imports what a PostgreSQL adapter must not.\ntype Fixture struct {\n\tRouter   *chi.Mux\n\tContract httpapi.Fixture\n}\n",
		"internal/" + capability + "/service.go": "// Package " + capability + " is a fixture.\npackage " + capability + "\n\nimport (\n\t\"github.com/go-chi/chi/v5\"\n\t\"github.com/jackc/pgx/v5\"\n\n\t\"" +
			module + "/internal/platform/config\"\n\t\"" + module + "/" + adapter + "/dbgen\"\n)\n\n// Fixture imports what business code must not.\ntype Fixture struct {\n\tRouter  *chi.Mux\n\tRows    pgx.Rows\n\tSetting config.Fixture\n\tQueries dbgen.Fixture\n}\n",
		"internal/" + capability + "/" + capability + "http/handler.go": "// Package " + capability + "http is a fixture.\npackage " + capability + "http\n\nimport (\n\t\"github.com/jackc/pgx/v5\"\n\n\t\"" +
			module + "/" + adapter + "\"\n)\n\n// Fixture imports what an HTTP adapter must not.\ntype Fixture struct {\n\tRows  pgx.Rows\n\tStore " + capability + "postgres.Fixture\n}\n",
	}
	for path, content := range files {
		if err := mkdirAllIn(fixture, filepath.Dir(path)); err != nil {
			t.Fatalf("create %s: %v", filepath.Dir(path), err)
		}
		if err := fixture.WriteFile(filepath.FromSlash(path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	return directory
}

// runDepguard reports which import each fixture file was denied, keyed by file
// and package.
func runDepguard(t *testing.T, linter string, fixture string) map[string]bool {
	t.Helper()

	// The repository's own configuration decides which linters run. Forcing
	// depguard on with --enable-only would keep this passing after someone
	// removes it from the enabled set, which is one edit away from every ban
	// being present and none being applied.
	command := exec.CommandContext(t.Context(), linter, "run", "./...")
	command.Dir = fixture
	output, err := command.CombinedOutput()
	// Findings make the linter exit non-zero, which is the expected outcome; a
	// failure to run at all is not, and its output is the only way to tell.
	if err != nil && !strings.Contains(string(output), "depguard") {
		t.Fatalf("run depguard: %v\n%s", err, output)
	}

	reported := make(map[string]bool)
	for _, line := range strings.Split(string(output), "\n") {
		file, rest, found := strings.Cut(line, ":")
		if !found || !strings.Contains(rest, "is not allowed from list") {
			continue
		}
		_, quoted, ok := strings.Cut(rest, "import '")
		if !ok {
			continue
		}
		pkg, _, ok := strings.Cut(quoted, "'")
		if !ok {
			continue
		}
		reported[filepath.ToSlash(file)+" "+pkg] = true
	}

	return reported
}

// mkdirAllIn creates a directory and its parents inside a root, which offers
// only single-level creation.
func mkdirAllIn(root *os.Root, directory string) error {
	if directory == "." {
		return nil
	}

	built := ""
	for _, element := range strings.Split(filepath.ToSlash(directory), "/") {
		built = filepath.Join(built, element)
		if err := root.Mkdir(built, 0o750); err != nil && !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("create %s: %w", built, err)
		}
	}

	return nil
}

func modulePath(t *testing.T) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(repositoryRoot, "..", "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		if path, found := strings.CutPrefix(line, "module "); found {
			return strings.TrimSpace(path)
		}
	}
	t.Fatal("go.mod declares no module path")

	return ""
}

package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The depguard rules in .golangci.yml match capabilities by layout, so a new
// module inherits the router, driver, and configuration bans without anyone
// editing the configuration. Two bans cannot be written that way: both name one
// capability's own PostgreSQL adapter, and depguard matches a denied package by
// prefix, which cannot express "any capability's adapter". Those two are
// therefore restated per capability, and a module added without them lints
// green while its boundary goes unenforced.
//
// The same prefixes carry the module path, which the first step of adopting
// this template replaces. A rename that misses .golangci.yml leaves every
// internal ban pointing at a package that no longer exists, and depguard
// reports nothing, because a ban that matches no import is not an error.

const (
	repositoryRoot = "../.."
	lintConfigPath = repositoryRoot + "/.golangci.yml"
	goModulePath   = repositoryRoot + "/go.mod"
)

// denyPattern matches the package of one depguard deny entry.
var denyPattern = regexp.MustCompile(`(?m)^\s*- pkg:\s*(\S+)`)

// modulePattern matches the module path declared by go.mod.
var modulePattern = regexp.MustCompile(`(?m)^module\s+(\S+)`)

// rulePattern matches the name of one depguard rule, which the configuration
// indents by eight spaces under the shared settings block.
var rulePattern = regexp.MustCompile(`(?m)^        (\S+):`)

func TestEveryCapabilityRestatesItsAdapterBans(t *testing.T) {
	t.Parallel()

	module := modulePath(t)
	businessBans := denials(t, "business-layer")
	httpBans := denials(t, "http-adapter")

	for _, capability := range capabilities(t) {
		t.Run(capability, func(t *testing.T) {
			t.Parallel()

			adapter := module + "/internal/" + capability + "/" + capability + "postgres"
			if hasPackage(t, capability, capability+"postgres/dbgen") && !businessBans[adapter+"/dbgen"] {
				t.Errorf("business-layer does not ban %s, so %s may import its generated queries directly", adapter+"/dbgen", capability)
			}
			if hasPackage(t, capability, capability+"http") && !httpBans[adapter] {
				t.Errorf("http-adapter does not ban %s, so its HTTP adapter may bypass the application service", adapter)
			}
		})
	}
}

func TestEveryInternalBanUsesTheCurrentModulePath(t *testing.T) {
	t.Parallel()

	module := modulePath(t)
	for _, rule := range ruleNames(t) {
		for banned := range denials(t, rule) {
			if !strings.Contains(banned, "/internal/") {
				continue
			}
			if !strings.HasPrefix(banned, module+"/") {
				t.Errorf("rule %s bans %s, which no longer belongs to module %s", rule, banned, module)
			}
		}
	}
}

// capabilities returns the capability directories under internal/, which are the
// ones owning an HTTP or PostgreSQL adapter named after them. Recognizing them
// by that layout keeps this guard from carrying a list of the directories that
// are not capabilities, which is the kind of restated list it exists to check.
func capabilities(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(repositoryRoot, "internal"))
	if err != nil {
		t.Fatalf("read internal packages: %v", err)
	}

	found := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if hasPackage(t, name, name+"http") || hasPackage(t, name, name+"postgres") {
			found = append(found, name)
		}
	}
	if len(found) == 0 {
		t.Fatal("no capability owns an adapter, so this guard would pass without checking anything")
	}

	return found
}

// hasPackage reports whether a capability owns the named child package.
func hasPackage(t *testing.T, capability string, child string) bool {
	t.Helper()

	info, err := os.Stat(filepath.Join(repositoryRoot, "internal", capability, child))
	if err != nil {
		return false
	}

	return info.IsDir()
}

// denials returns the packages one depguard rule bans.
func denials(t *testing.T, rule string) map[string]bool {
	t.Helper()

	banned := make(map[string]bool)
	for _, match := range denyPattern.FindAllStringSubmatch(ruleSection(t, rule), -1) {
		banned[match[1]] = true
	}
	if len(banned) == 0 {
		t.Fatalf("depguard rule %s bans nothing", rule)
	}

	return banned
}

// ruleSection returns the configured body of one depguard rule, which ends where
// the next rule at the same indentation begins.
func ruleSection(t *testing.T, rule string) string {
	t.Helper()

	declaration := "\n        " + rule + ":"
	index := strings.Index(lintConfig(t), declaration)
	if index < 0 {
		t.Fatalf("depguard declares no rule %s", rule)
	}

	body := lintConfig(t)[index+len(declaration):]
	if next := rulePattern.FindStringIndex(body); next != nil {
		return body[:next[0]]
	}

	return body
}

// ruleNames returns every depguard rule the configuration declares.
func ruleNames(t *testing.T) []string {
	t.Helper()

	config := lintConfig(t)
	start := strings.Index(config, "\n    depguard:")
	if start < 0 {
		t.Fatal("the lint configuration declares no depguard settings")
	}

	names := make([]string, 0)
	for _, match := range rulePattern.FindAllStringSubmatch(config[start:], -1) {
		names = append(names, match[1])
	}
	if len(names) == 0 {
		t.Fatal("depguard declares no rules")
	}

	return names
}

func lintConfig(t *testing.T) string {
	t.Helper()

	content, err := os.ReadFile(lintConfigPath)
	if err != nil {
		t.Fatalf("read lint configuration: %v", err)
	}

	return string(content)
}

func modulePath(t *testing.T) string {
	t.Helper()

	content, err := os.ReadFile(goModulePath)
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	match := modulePattern.FindStringSubmatch(string(content))
	if match == nil {
		t.Fatal("go.mod declares no module path")
	}

	return match[1]
}

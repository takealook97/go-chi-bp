#!/bin/sh
# Turn this template into a project: rename the module path, and optionally the
# example capability, everywhere they appear.
#
# Doing this by hand is the step most likely to go wrong, and the failure is
# quiet. The depguard rules in .golangci.yml name the module path literally, and
# a rule that matches no import is not an error: lint keeps reporting zero
# issues while the architecture boundaries it enforces are gone. This script
# changes every occurrence at once and then verifies that none are left.
set -eu

# Glob ranges such as [a-z] are collation-dependent: in most locales the order
# interleaves cases, so [!a-z] does not reject an uppercase letter and the
# validation below would pass a name it is meant to refuse. C collation also
# keeps sort, tr, and grep behaving the same way on every machine.
LC_ALL=C
export LC_ALL

usage() {
	cat <<'USAGE'
Usage: scripts/init-project.sh <module-path> [<capability> <capability-plural>]

  module-path        The Go module path of the new project,
                     for example github.com/acme/billing.
  capability         Singular name replacing the "widget" example,
                     for example invoice. Lowercase ASCII letters only.
  capability-plural  Its plural, for example invoices. It is an argument
                     because it names a table and a route, and no script
                     should guess English inflection.

Examples:
  scripts/init-project.sh github.com/acme/billing
  scripts/init-project.sh github.com/acme/billing invoice invoices
USAGE
}

PLACEHOLDER_MODULE='github.com/lukuku-dev/go-chi-bp'

if [ $# -ne 1 ] && [ $# -ne 3 ]; then
	usage >&2
	exit 2
fi

MODULE="$1"
CAPABILITY="${2:-}"
CAPABILITY_PLURAL="${3:-}"

cd "$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)"

# Set once the script starts rewriting files, so that a failure part of the way
# through says how to get back rather than leaving a half-renamed tree.
MUTATED=no

fail() {
	echo "error: $1" >&2
	if [ "$MUTATED" = yes ]; then
		echo "the repository was partly rewritten; restore it with:" >&2
		echo "  git reset --hard && git clean -fd" >&2
	fi
	exit 1
}

case "$MODULE" in
*[!A-Za-z0-9./_-]* | '' | */) fail "module path $MODULE is not a valid Go module path" ;;
*/*) ;;
*) fail "module path $MODULE must contain a host, for example github.com/acme/billing" ;;
esac
if [ "$MODULE" = "$PLACEHOLDER_MODULE" ]; then
	fail "module path is already $PLACEHOLDER_MODULE; choose the new project's path"
fi
if [ -n "$CAPABILITY" ]; then
	for name in "$CAPABILITY" "$CAPABILITY_PLURAL"; do
		case "$name" in
		*[!a-z]* | '') fail "capability name $name must be one or more lowercase ASCII letters" ;;
		esac
	done
	if [ "$CAPABILITY" = "$CAPABILITY_PLURAL" ]; then
		fail "the plural must differ from the singular; it names the table and the route"
	fi
	if [ "$CAPABILITY" = widget ]; then
		fail "widget is the name being replaced; choose the capability this project is about"
	fi
	# The capability becomes internal/<capability>, so a name already taken there
	# would make the rename collide with a directory that is not a capability.
	for reserved in app httpapi platform testkit; do
		if [ "$CAPABILITY" = "$reserved" ]; then
			fail "$reserved is already a directory under internal/; choose another capability name"
		fi
	done
fi

# Everything below reads and rewrites the repository through git.
if ! git rev-parse --git-dir >/dev/null 2>&1; then
	fail "this is not a Git repository, and the rename is applied through git"
fi

# A dirty worktree makes this irreversible in practice: the rename touches most
# files, so discarding it would take unrelated work along.
if [ -n "$(git status --porcelain)" ]; then
	fail "the working tree has uncommitted changes; commit or stash them first"
fi
if ! grep -q "^module $PLACEHOLDER_MODULE\$" go.mod; then
	fail "go.mod is not the template's module, so this repository is already initialized"
fi

# Everything except this script, which holds the placeholder deliberately, and
# LICENSE, whose text is not the project's to rewrite.
tracked_files() {
	git ls-files | grep -v -e '^scripts/init-project\.sh$' -e '^LICENSE$'
}

# Newline-separated on purpose: this operates on one known file list, and -z is
# a GNU grep extension that the BSD grep on macOS does not have.
replace_everywhere() {
	from="$1"
	to="$2"
	files="$(tracked_files | xargs grep -l -F -e "$from" || true)"
	# GNU xargs runs the command once with no arguments when its input is empty,
	# which would leave sed reading the terminal.
	[ -n "$files" ] || return 0
	printf '%s\n' "$files" | xargs sed -i.initbak "s#$from#$to#g"
	find . -name '*.initbak' -delete
}

echo "Setting the module path to $MODULE"
MUTATED=yes
go mod edit -module "$MODULE"
replace_everywhere "$PLACEHOLDER_MODULE" "$MODULE"

if [ -n "$CAPABILITY" ]; then
	echo "Renaming the widget example to $CAPABILITY/$CAPABILITY_PLURAL"

	capitalize() {
		printf '%s%s' "$(printf '%s' "$1" | cut -c1 | tr '[:lower:]' '[:upper:]')" "$(printf '%s' "$1" | cut -c2-)"
	}
	Capability="$(capitalize "$CAPABILITY")"
	CapabilityPlural="$(capitalize "$CAPABILITY_PLURAL")"

	# Plural before singular, because the singular is a prefix of it. Applied in
	# this order the four substitutions also cover the compound identifiers:
	# widgethttp, widgetID, WidgetList, listWidgets, widgets_created_at_id_idx.
	replace_everywhere 'Widgets' "$CapabilityPlural"
	replace_everywhere 'Widget' "$Capability"
	replace_everywhere 'widgets' "$CAPABILITY_PLURAL"
	replace_everywhere 'widget' "$CAPABILITY"

	# Paths are renamed by the same four substitutions rather than from a list,
	# so a file or directory added to the example capability later is carried
	# along instead of being left behind under its old name.
	rename_component() {
		printf '%s' "$1" | sed \
			-e "s#Widgets#$CapabilityPlural#g" \
			-e "s#Widget#$Capability#g" \
			-e "s#widgets#$CAPABILITY_PLURAL#g" \
			-e "s#widget#$CAPABILITY#g"
	}

	git ls-files | while IFS= read -r path; do
		base="$(basename "$path")"
		renamed="$(rename_component "$base")"
		if [ "$renamed" != "$base" ]; then
			git mv "$path" "$(dirname "$path")/$renamed"
		fi
	done

	# Deepest first, because renaming a parent invalidates every path below it.
	find . -type d \( -name '*widget*' -o -name '*Widget*' \) -not -path './.git/*' |
		awk '{ print length, $0 }' | sort -rn | cut -d' ' -f2- |
		while IFS= read -r directory; do
			base="$(basename "$directory")"
			git mv "$directory" "$(dirname "$directory")/$(rename_component "$base")"
		done
fi

echo "Regenerating contract and database code (builds the pinned tools on a first run)"
make sqlc openapi >/dev/null
gofmt -w .

echo "Verifying that nothing was missed"

# grep -l exits 1 when nothing matches, which is this search's success case, so
# the exit status cannot be trusted here. Standard error is left alone: the only
# other reason grep fails is a path it could not read, and that must be seen.
search_tracked() {
	tracked_files | xargs grep -l -F "$@" || true
}

# A search that reaches no file reports no leftovers and looks like success.
# That is not hypothetical: this check was once written with `xargs -0` against a
# newline-separated list, which handed grep the whole list as one filename and
# made the verification below incapable of ever failing. Every file the rename
# touched names the new module, so finding none of them means the search itself
# is broken rather than that the tree is clean.
if [ -z "$(search_tracked -e "$MODULE")" ]; then
	fail "the search found no file naming $MODULE, so it is not reading the repository"
fi

if [ -n "$CAPABILITY" ]; then
	leftovers="$(search_tracked -e "$PLACEHOLDER_MODULE" -e widget -e Widget)"
else
	leftovers="$(search_tracked -e "$PLACEHOLDER_MODULE")"
fi
if [ -n "$leftovers" ]; then
	fail "these files still hold template names:
$leftovers"
fi
go build ./... >/dev/null

cat <<DONE

Done. The module is $MODULE${CAPABILITY:+ and the capability is $CAPABILITY}.

Next:
  1. Put your own copyright holder in LICENSE. It still names this template's
     author, which is the one thing here that is not yours to inherit.
  2. Replace the reporting paragraph in SECURITY.md, which is a placeholder.
  3. Trim the template sections from README.md: "Create a project from this
     template" describes this script, not your project.
  4. Run: make check
DONE

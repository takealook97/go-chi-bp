#!/bin/sh
set -eu

usage() {
  echo "usage: $0 <commit-message-file> | --subject <subject>" >&2
  exit 2
}

if [ "${1:-}" = "--subject" ]; then
  [ "$#" -eq 2 ] || usage
  subject=$2
elif [ "$#" -eq 1 ]; then
  subject=$(sed -n '1p' "$1")
else
  usage
fi

if [ "${#subject}" -gt 72 ]; then
  echo "commit subject must not exceed 72 characters" >&2
  exit 1
fi

pattern='^(feat|fix|refactor|docs|test|build|ci|chore|perf|security|revert): [a-z][a-z0-9].*$'
if ! printf '%s\n' "$subject" | grep -Eq "$pattern"; then
  echo "invalid commit subject: $subject" >&2
  echo "expected English 'type: imperative summary' format" >&2
  exit 1
fi

case "$subject" in
  *.)
    echo "commit subject must not end with a period" >&2
    exit 1
    ;;
esac

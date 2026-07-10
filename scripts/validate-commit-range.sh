#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: $0 <base-revision> <head-revision>" >&2
  exit 2
fi

base=$1
head=$2
failed=0

for commit in $(git rev-list --reverse "$base..$head"); do
  subject=$(git show -s --format='%s' "$commit")
  if ! "$(dirname "$0")/validate-commit-msg.sh" --subject "$subject"; then
    failed=1
  fi
done

exit "$failed"

#!/bin/sh

# Attach this terminal to the tmux session the browser tab named.
#
# The name arrives from a URL query string, so it is input from the browser and
# is checked as such: letters, digits, underscore and hyphen, 1 to 32 of them.
# That is the whole of what tmux needs, and it keeps the name out of every
# character tmux gives a meaning to in a target (`:` and `.`).
#
# A `case` with a negated class rather than grep, because grep matches line by
# line and would accept a name with a newline in it as long as one line passed.

set -eu

if [ "$#" -gt 1 ]; then
  echo "console: expected one session name, got $#." >&2
  exit 2
fi

name=${1:-main}

case $name in
  *[!A-Za-z0-9_-]*)
    echo "console: '$name' is not a valid session name; use letters, digits, _ and -." >&2
    exit 2
    ;;
esac

if [ "${#name}" -gt 32 ]; then
  echo "console: session names are at most 32 characters." >&2
  exit 2
fi

exec tmux new-session -A -s "$name"

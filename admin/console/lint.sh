#!/usr/bin/env bash

# Check the console's scripts, and that the image carries what they reach for.
#
# A program missing from the image does not fail anything until somebody opens a
# terminal: entrypoint.sh exec'ing a ttyd that is not there is a crash-loop with
# "not found", and a .zshrc sourcing a plugin path that moved is a line of red in
# every new session. So every program and file the scripts and dotfiles name has
# to appear in the Dockerfile's build-time check, which fails the BUILD when one
# is absent — and this checks that list against what is actually used.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

shellcheck entrypoint.sh attach.sh lint.sh smoke.sh

problems=()

# The paths the Dockerfile's verification RUN tests for. Read from the `for`
# loops of `test -x`/`test -f` rather than from the whole file, because a path
# that only appears in a comment or a COPY is not one anything checks.
checked=$(awk '
  /^RUN set -eu; \\$/ { block = 1; buf = ""; next }
  block { buf = buf $0 "\n"; if ($0 !~ /\\$/) { if (buf ~ /test -[xf]/) printf "%s", buf; block = 0 } }
' Dockerfile | grep -oE '/[A-Za-z0-9_./+-]+' | sort -u)

if [[ -z "$checked" ]]; then
  problems+=("found no build-time binary check in the Dockerfile; this lint would pass on anything")
fi

is_checked_program() {
  grep -qE "/$1\$" <<<"$checked"
}

is_checked_file() {
  grep -qxF "$1" <<<"$checked"
}

# --- programs the scripts and dotfiles run -------------------------------------
#
# `exec X` in the scripts, `command -v X`, `source <(X ...)` and `$(X ...)` in the
# .zshrc, and the default-shell in .tmux.conf. Shell builtins and functions are
# excluded by name; anything else these patterns catch is a program.
used=$(
  {
    grep -hoE '^[[:space:]]*exec [A-Za-z0-9_/.-]+' entrypoint.sh attach.sh | awk '{print $2}'
    grep -hoE 'command -v [A-Za-z0-9_-]+' rootfs/etc/skel-lab/.zshrc | awk '{print $3}'
    grep -hoE 'source <\([A-Za-z0-9_-]+' rootfs/etc/skel-lab/.zshrc | sed 's/.*(//'
    grep -hoE '[$][(][A-Za-z0-9_-]+' rootfs/etc/skel-lab/.zshrc | cut -c3-
    grep -hoE '^set -g default-shell [^ ]+' rootfs/etc/skel-lab/.tmux.conf | awk '{print $4}'
  } | sed 's#.*/##' | sort -u
)

builtins=" id print _console_kube_context "
for program in $used; do
  [[ "$builtins" == *" $program "* ]] && continue
  is_checked_program "$program" ||
    problems+=("$program is used by the scripts or dotfiles but the Dockerfile never verifies it")
done

# Also the ones that ARE the product, which nothing above runs by name: an image
# called a console that ships without ssh or k9s is broken even if every script
# still works.
for program in zsh tmux ttyd ssh git curl jq bash less vim kubectl helm k9s attach.sh entrypoint.sh; do
  is_checked_program "$program" ||
    problems+=("$program is part of the console but the Dockerfile never verifies it")
done

# --- files the dotfiles source --------------------------------------------------
while read -r file; do
  is_checked_file "$file" ||
    problems+=(".zshrc sources $file, which the Dockerfile never verifies")
done < <(grep -hoE '^source /[^ ]+' rootfs/etc/skel-lab/.zshrc | awk '{print $2}')

if grep -q 'tmux-256color' rootfs/etc/skel-lab/.tmux.conf; then
  is_checked_file /etc/terminfo/t/tmux-256color ||
    problems+=(".tmux.conf asks for tmux-256color, whose terminfo the Dockerfile never verifies")
fi

# --- the contracts the gate and the chart rely on --------------------------------
if ! grep -qE '^exec ttyd -i 127\.0\.0\.1 -p 7681 ' entrypoint.sh; then
  problems+=("entrypoint.sh no longer serves ttyd on 127.0.0.1:7681, which the gate dials")
fi

if ((${#problems[@]})); then
  echo "The console image has problems:" >&2
  printf '  - %s\n' "${problems[@]}" >&2
  exit 1
fi

echo "Console image validated: $(wc -w <<<"$used" | tr -d ' ') programs used, all verified at build time."

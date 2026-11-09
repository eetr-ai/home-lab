#!/bin/sh

# Smoke checks, run INSIDE the built image: `task admin-console:test` pipes this
# file to `sh -s` in the container rather than mounting it, so it also works with
# a Docker context on another machine.
#
# Each check is something that has broken or could: a tool missing or the wrong
# architecture, a session name that should have been refused, an entrypoint that
# carried on into ttyd with a home it cannot write to.

set -eu

failures=0
pass() { echo "ok   $1"; }
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

check() {
  label=$1
  shift
  if out=$("$@" 2>&1); then
    pass "$label: $(printf '%s' "$out" | head -n 1)"
  else
    fail "$label: $out"
  fi
}

# --- the tools run, which also proves they are the right architecture ---------
check zsh zsh --version
check tmux tmux -V
check ttyd ttyd --version
check kubectl kubectl version --client
check helm helm version --short
check k9s k9s version --short
check ssh ssh -V
check git git --version
# What tcell runs when k9s starts under tmux; without an answer k9s will not draw.
check "infocmp tmux-256color" infocmp tmux-256color

# --- attach.sh refuses what it should ------------------------------------------
refuses() {
  label=$1
  shift
  if out=$(/usr/local/bin/attach.sh "$@" 2>&1); then
    fail "attach.sh accepted $label"
  elif [ $? -ne 2 ]; then
    fail "attach.sh refused $label with the wrong status: $out"
  # The message as well as the status, because a shell syntax error exits 2 too
  # — which is how a broken attach.sh once passed this check.
  elif ! printf '%s' "$out" | grep -q '^console: '; then
    fail "attach.sh refused $label without saying why: $out"
  else
    pass "attach.sh refuses $label"
  fi
}

refuses "bad;name" "bad;name"
refuses "a name with a space" "two words"
refuses "a name with a colon" "main:0"
refuses "a name with a newline" "$(printf 'ok\nbad;name')"
refuses "33 characters" "abcdefghijklmnopqrstuvwxyz0123456"
refuses "two arguments" one two

# --- the entrypoint on a writable home -----------------------------------------
# Started in the background and probed over loopback, so this proves the skel
# copy, ~/.ssh, and that ttyd actually listens where the gate will dial.
home=$(mktemp -d)
HOME=$home /usr/local/bin/entrypoint.sh >/dev/null 2>&1 &
pid=$!
listening=false
for _ in 1 2 3 4 5 6 7 8 9 10; do
  if curl -fs -o /dev/null http://127.0.0.1:7681/; then
    listening=true
    break
  fi
  sleep 0.5
done
kill "$pid" 2>/dev/null || true
wait "$pid" 2>/dev/null || true

if $listening; then pass "ttyd listens on 127.0.0.1:7681"; else fail "ttyd never answered on 127.0.0.1:7681"; fi
if [ -f "$home/.zshrc" ] && [ -f "$home/.tmux.conf" ]; then pass "skel files copied"; else fail "skel files were not copied"; fi
if [ "$(stat -c %a "$home/.ssh" 2>/dev/null)" = 700 ]; then pass ".ssh is 0700"; else fail ".ssh is missing or not 0700"; fi
if [ "$(HOME=$home kubectl config current-context 2>&1)" = home-lab ]; then
  pass "kubeconfig names the in-cluster context"
else
  fail "no home-lab context in ~/.kube/config: $(HOME=$home kubectl config current-context 2>&1)"
fi
# The name alone is not enough: a context pointing at a cluster or user that is
# not in the file still prints its name, and k9s would get nothing from it.
# --minify keeps only what the current context resolves to.
resolved=$(HOME=$home kubectl config view --minify -o jsonpath='{.clusters[0].name} {.users[0].name}' 2>&1 || true)
if [ "$resolved" = "home-lab admin-console" ]; then
  pass "the home-lab context resolves its cluster and user"
else
  fail "the home-lab context does not resolve its cluster and user: $resolved"
fi
# Without KUBERNETES_SERVICE_HOST, which `docker run` does not set, the DNS name
# stays as the fallback.
server=$(HOME=$home kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' 2>&1 || true)
if [ "$server" = https://kubernetes.default.svc ]; then
  pass "kubeconfig falls back to the cluster DNS name outside a cluster"
else
  fail "kubeconfig server outside a cluster is $server"
fi
if ls "$home/.kube"/config.tmp.* >/dev/null 2>&1; then fail "entrypoint left a temporary kubeconfig behind"; else pass "no temporary kubeconfig left behind"; fi

# ...and an edited dotfile or kubeconfig survives a restart.
echo "# mine" >"$home/.zshrc"
echo "# mine" >"$home/.kube/config"
HOME=$home timeout 3 /usr/local/bin/entrypoint.sh >/dev/null 2>&1 || true
if [ "$(cat "$home/.zshrc")" = "# mine" ]; then pass "existing dotfiles are left alone"; else fail "entrypoint overwrote an existing .zshrc"; fi
if [ "$(cat "$home/.kube/config")" = "# mine" ]; then pass "an existing kubeconfig is left alone"; else fail "entrypoint overwrote an existing kubeconfig"; fi

# A volume kubectl has already used has ~/.kube with only a cache in it; the
# kubeconfig still has to arrive there.
rm -f "$home/.kube/config"
mkdir -p "$home/.kube/cache"
HOME=$home timeout 3 /usr/local/bin/entrypoint.sh >/dev/null 2>&1 || true
if [ -f "$home/.kube/config" ]; then pass "kubeconfig lands in an existing ~/.kube"; else fail "kubeconfig skipped because ~/.kube existed"; fi

# In a pod, the server is the address Kubernetes injects, so the console does not
# depend on cluster DNS. Documentation addresses (RFC 5737, RFC 3849) stand in.
server_for() {
  rm -f "$home/.kube/config"
  HOME=$home KUBERNETES_SERVICE_HOST=$1 KUBERNETES_SERVICE_PORT=$2 \
    timeout 3 /usr/local/bin/entrypoint.sh >/dev/null 2>&1 || true
  HOME=$home kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' 2>&1 || true
}
server=$(server_for 192.0.2.1 6443)
if [ "$server" = https://192.0.2.1:6443 ]; then pass "kubeconfig dials the service address, not DNS"; else fail "kubeconfig server in a pod is $server"; fi
server=$(server_for 2001:db8::1 443)
if [ "$server" = "https://[2001:db8::1]:443" ]; then pass "kubeconfig brackets an IPv6 service address"; else fail "kubeconfig server for IPv6 is $server"; fi

# The default .zshrc loads cleanly. Anything on stderr is a broken line.
cp /etc/skel-lab/.zshrc "$home/.zshrc"
if err=$(HOME=$home zsh -i -c exit 2>&1 >/dev/null) && [ -z "$err" ]; then
  pass ".zshrc loads without errors"
else
  fail ".zshrc printed errors: $err"
fi

# --- the entrypoint on a read-only home ----------------------------------------
# chmod rather than a read-only mount, because this runs as a non-root user that
# cannot mount anything. Under timeout so a regression hangs for seconds, not
# forever, if it carries on into ttyd.
chmod 0555 "$home"
if out=$(HOME=$home timeout 5 /usr/local/bin/entrypoint.sh 2>&1); then
  fail "entrypoint started on a read-only home"
elif printf '%s' "$out" | grep -q "is not writable"; then
  pass "entrypoint refuses a read-only home"
else
  fail "entrypoint failed on a read-only home without saying why: $out"
fi
chmod 0755 "$home"
rm -rf "$home"

if [ "$failures" -ne 0 ]; then
  echo "$failures check(s) failed" >&2
  exit 1
fi
echo "all console smoke checks passed"

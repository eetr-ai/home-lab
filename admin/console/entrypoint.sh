#!/bin/sh

# Prepare the home directory, then serve the terminal.
#
# The home directory is the PVC, so it starts empty on a new volume and keeps
# whatever the operator changed on every one after that. Hence "copy only when
# missing": a dotfile the operator edited is theirs, and an image upgrade must not
# put ours back over it.

set -eu

# Checked up front because every failure downstream of an unwritable home is
# confusing — zsh starts without history, compinit cannot write its dump, ssh
# has no ~/.ssh to keep keys in — and none of them says why.
#
# The probe runs in a subshell: a failed redirection on `:` is fatal to a POSIX
# shell, so in-line it would exit right here with no message at all.
probe="$HOME/.console-write-test.$$"
if ! (: >"$probe") 2>/dev/null; then
  echo "console: HOME ($HOME) is not writable by uid $(id -u)." >&2
  echo "console: mount the home volume there and make it writable for this user (fsGroup 65532)." >&2
  exit 1
fi
rm -f "$probe"

for src in /etc/skel-lab/.[!.]* /etc/skel-lab/*; do
  # An unmatched glob comes back as itself.
  [ -e "$src" ] || continue
  dest="$HOME/${src##*/}"
  [ -e "$dest" ] || cp -R "$src" "$dest"
done

mkdir -p "$HOME/.ssh"
chmod 700 "$HOME/.ssh"

# By file rather than with the skeleton: kubectl makes ~/.kube for its cache the
# first time it runs, so a directory copy would be skipped on every volume that
# has used kubectl -- which is every volume that existed before this did.
if [ ! -e "$HOME/.kube/config" ]; then
  mkdir -p "$HOME/.kube"
  cp /etc/console/in-cluster.yaml "$HOME/.kube/config"
  chmod 600 "$HOME/.kube/config"
fi

# Loopback and 7681 are contracts: the gate container dials exactly that, and the
# chart test asserts it. -W makes the terminal writable, -a lets each tab pass its
# session name as ?arg=, which ttyd appends to attach.sh's argv.
exec ttyd -i 127.0.0.1 -p 7681 -W -a -t disableLeaveAlert=true /usr/local/bin/attach.sh

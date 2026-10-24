# The console

A terminal in a pod, for managing the whole lab from the panel's Console page.

This image is the `shell` container of the admin-console pod. It runs
[ttyd](https://github.com/tsl0922/ttyd) on loopback; the pod's `gate` container —
built from admin-web's image — authorizes the browser's WebSocket upgrade and
pipes the raw TCP to it. Each browser tab names a tmux session, so a reload or a
second tab lands back in the same shell rather than a new one.

```text
Dockerfile         alpine, the shell tools, and pinned, checksummed kubectl/helm/k9s
entrypoint.sh      prepares the home directory, then execs ttyd
attach.sh          what ttyd runs per connection: validate the name, attach to tmux
rootfs/etc/skel-lab/  the default .zshrc and .tmux.conf, copied into home once
lint.sh            shellcheck, and "the image carries what the scripts use"
smoke.sh           checks run inside the built image by `task test`
```

The pod's ServiceAccount is cluster-admin, so what is installed is what that job
needs: `kubectl` (the cluster's 1.36 minor), `helm` (the repository's pin), `k9s`,
`ssh`, `git`, `curl`, `jq`, `vim`, under zsh and tmux.

## Contracts

These are relied on from outside this directory. Change one and you change the
gate and the chart with it.

| | |
| --- | --- |
| **Port** | ttyd listens on **7681**. The gate dials exactly that. |
| **Loopback only** | ttyd binds **127.0.0.1**, never `0.0.0.0`. The terminal has no authentication of its own; the gate is the only way in, and that holds only while nothing else in the pod's network can reach the port. The chart test asserts it. |
| **Session names** | The tab's session name arrives as ttyd's URL argument (`?arg=<name>`, enabled by `-a`). `attach.sh` accepts `^[A-Za-z0-9_-]{1,32}$`, uses `main` when there is none, and otherwise exits 2 with a message. The gate should refuse the same set before it ever reaches here. |
| **Home volume** | `/home/lab` is the PVC: history, `~/.ssh`, dotfiles. It must be writable by uid/gid **65532** (`fsGroup: 65532`); the entrypoint fails loudly if it is not, rather than starting a shell that quietly loses everything. Dotfiles from `/etc/skel-lab` are copied only when missing, so an operator's edits survive upgrades. |
| **/tmp** | tmux keeps its socket in `/tmp`. With `readOnlyRootFilesystem`, mount an `emptyDir` there. |

All tmux sessions live in the one container, so they last as long as the pod does,
not as long as the tab. Restarting the pod ends them; the home volume keeps the
history.

## Tasks

```bash
task lint    # shellcheck, and every program the scripts use is verified in the Dockerfile
task test    # build, then run smoke.sh inside the image
task build   # CONSOLE_IMAGE / CONSOLE_TAG to name it
```

`task test` checks that each tool runs (which also proves its architecture), that
`attach.sh` refuses names it should, that the entrypoint copies the skeleton,
leaves an edited dotfile alone and gets ttyd listening on 127.0.0.1:7681, that the
default `.zshrc` loads without errors, and that a read-only home is refused with a
message.

## Moving a version

The kubectl, helm and k9s versions and their sha256 sums are build arguments at
the top of the Dockerfile, with the URL each sum comes from. Replace the version
and both architectures' sums together; a mismatch fails the build.

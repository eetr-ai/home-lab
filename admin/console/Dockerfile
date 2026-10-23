# syntax=docker/dockerfile:1
#
# The admin panel's console: a terminal in a pod, for managing the whole lab.
#
# This is the `shell` half of the admin-console pod. ttyd serves a terminal on
# loopback only, and the pod's `gate` container — admin-web's image — is the one
# thing that may reach it: it authorizes the WebSocket upgrade and pipes the raw
# TCP to 127.0.0.1:7681. Every browser tab names a session, ttyd hands that name
# to attach.sh, and attach.sh attaches to (or creates) the tmux session of that
# name, so a reload or a second tab lands back where the operator left off.
#
# The pod's ServiceAccount is cluster-admin, so the tools for that job ship here:
# kubectl, helm and k9s, downloaded at pinned versions and checked against their
# published sha256 sums rather than taken from whatever the package index has.

FROM alpine:3.22

# Set by buildx for the platform being built; the downloads below pick their
# architecture from it. Declared without a default on purpose: a plain
# `docker build` sets it too, and a default here would only be a way to build an
# amd64 kubectl into an arm64 image.
ARG TARGETARCH

# zsh and its two plugins are the shell; tmux is what keeps a session alive
# between tabs; ttyd serves it. openssh-client because the PVC carries ~/.ssh and
# the lab's hosts are reached that way. ncurses-terminfo-base for tmux-256color,
# which .tmux.conf asks for and tmux does not bring by itself.
RUN apk add --no-cache \
      zsh zsh-autosuggestions zsh-syntax-highlighting \
      tmux ttyd ncurses-terminfo-base \
      openssh-client git curl jq bash less vim \
      ca-certificates tzdata

# kubectl at the newest 1.36 patch, because the cluster is 1.36 and kubectl
# supports one minor either side; helm at the repository's pin (Taskfile.yml's
# HELM_VERSION), so the chart the console installs is rendered by the same helm
# CI renders it with. To move one, replace its version and both sums:
#   kubectl  https://dl.k8s.io/release/<v>/bin/linux/<arch>/kubectl.sha256
#   helm     https://get.helm.sh/helm-<v>-linux-<arch>.tar.gz.sha256sum
#   k9s      https://github.com/derailed/k9s/releases/download/<v>/checksums.sha256
ARG KUBECTL_VERSION=v1.36.5
ARG KUBECTL_SHA256_AMD64=33bc88a24c3b09cf55bfd59e6ca977881b61fc3f25ab65bc80c65b430248648e
ARG KUBECTL_SHA256_ARM64=88fc1aca8fd0c1b44fe379f67c39bd6858bdfc50241dc55775f84d0f5b31da3a
ARG HELM_VERSION=v4.1.1
ARG HELM_SHA256_AMD64=5d4c7623283e6dfb1971957f4b755468ab64917066a8567dd50464af298f4031
ARG HELM_SHA256_ARM64=02a5fb7742469d2d132e24cb7c3f52885894043576588c6788b6813297629edd
ARG K9S_VERSION=v0.51.0
ARG K9S_SHA256_AMD64=c3752ad51a5a4015a113819c4eeb6e55a4d0e4b8e652494797532f6fc8161dd7
ARG K9S_SHA256_ARM64=3ee05c82e5f9198928a4e86133608ba6a2c10a2244d6a7789e820f78319d640c

# One RUN, so a sum that does not match fails the build before anything lands in a
# layer. The case refuses an architecture we have no sums for rather than
# downloading a binary nothing checked.
RUN set -eu; \
    case "$TARGETARCH" in \
      amd64) kubectl_sum=$KUBECTL_SHA256_AMD64; helm_sum=$HELM_SHA256_AMD64; k9s_sum=$K9S_SHA256_AMD64 ;; \
      arm64) kubectl_sum=$KUBECTL_SHA256_ARM64; helm_sum=$HELM_SHA256_ARM64; k9s_sum=$K9S_SHA256_ARM64 ;; \
      *) echo "no pinned checksums for TARGETARCH='$TARGETARCH'" >&2; exit 1 ;; \
    esac; \
    work=$(mktemp -d); cd "$work"; \
    curl -fsSLo kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl"; \
    echo "$kubectl_sum  kubectl" | sha256sum -c -; \
    install -m 0755 kubectl /usr/local/bin/kubectl; \
    curl -fsSLo helm.tar.gz "https://get.helm.sh/helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz"; \
    echo "$helm_sum  helm.tar.gz" | sha256sum -c -; \
    tar -xzf helm.tar.gz; \
    install -m 0755 "linux-${TARGETARCH}/helm" /usr/local/bin/helm; \
    curl -fsSLo k9s.tar.gz "https://github.com/derailed/k9s/releases/download/${K9S_VERSION}/k9s_Linux_${TARGETARCH}.tar.gz"; \
    echo "$k9s_sum  k9s.tar.gz" | sha256sum -c -; \
    tar -xzf k9s.tar.gz k9s; \
    install -m 0755 k9s /usr/local/bin/k9s; \
    cd /; rm -rf "$work"

# uid 65532, the same non-root user every other admin image runs as. Named `lab`
# here because a person reads it in the prompt, and its home is the PVC.
RUN addgroup -g 65532 -S lab \
    && adduser -u 65532 -S -G lab -h /home/lab -s /bin/zsh lab

COPY rootfs/ /
COPY --chmod=0755 entrypoint.sh /usr/local/bin/entrypoint.sh
COPY --chmod=0755 attach.sh /usr/local/bin/attach.sh

# Fail the build rather than the pod if a program the scripts or .zshrc reach for
# is not where they expect. lint.sh checks this list against what they use, so a
# new tool in .zshrc that nobody added here fails `task lint` instead of printing
# "command not found" into somebody's first session.
RUN set -eu; \
    for bin in \
      /bin/zsh /usr/bin/tmux /usr/bin/ttyd /usr/bin/ssh /usr/bin/git \
      /usr/bin/curl /usr/bin/jq /bin/bash /usr/bin/less /usr/bin/vim \
      /usr/local/bin/kubectl /usr/local/bin/helm /usr/local/bin/k9s \
      /usr/local/bin/entrypoint.sh /usr/local/bin/attach.sh; do \
      test -x "$bin" || { echo "missing: $bin"; exit 1; }; \
    done; \
    for file in \
      /usr/share/zsh/plugins/zsh-autosuggestions/zsh-autosuggestions.zsh \
      /usr/share/zsh/plugins/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh \
      /etc/terminfo/t/tmux-256color; do \
      test -f "$file" || { echo "missing: $file"; exit 1; }; \
    done

# Owned here so the image works with no volume at all; with one, the PVC is
# mounted over it and the chart's fsGroup gives it the same owner.
RUN chown 65532:65532 /home/lab && chmod 0750 /home/lab

ENV HOME=/home/lab \
    SHELL=/bin/zsh \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8

WORKDIR /home/lab
USER 65532:65532

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]

# syntax=docker/dockerfile:1
#
# The admin panel's agent.
#
# Octo publishes a standalone runtime image, and it is distroless
# (gcr.io/distroless/static-debian12:nonroot) — one static binary, no shell, no
# package manager. That is right for an integration that serves a webhook and
# wrong for this one: a `cli-run` block resolves its allow list when the flow is
# BUILT, so a tool naming curl in an image with no curl fails the whole config on
# load rather than the first call that would have used it.
#
# So this image is the published runtime's binary on a real userland. It is not
# `FROM juancavallotti/octo-runtime` — there is no apk there to add anything with —
# it is a build stage we copy the binary out of, which means no Go toolchain and no
# compile step here.
#
# Octo also publishes an `octo-agenticrunner` that already carries these programs.
# It cannot be used: its entrypoint is the k8s-tagged build, which exits without a
# runtime services module, so it needs an orchestrator and a NATS this cluster does
# not run.
#
# Pinned by digest rather than by tag, because tags move and this one supplies the
# binary the pod actually runs. The version is in the comment and not in a second
# build argument: two things naming the same image is two things that can disagree,
# and only one of them would be the one Docker pulls.
#
# octo v0.8.8. To move it, read the new digest from Docker Hub and replace both:
#   docker buildx imagetools inspect juancavallotti/octo-runtime:<version>
ARG OCTO_DIGEST=sha256:42df97d040ee8af0c3f96c63c5f3c5f1f7ea0c10a6a53af82bc6cc04a8d16f55
FROM juancavallotti/octo-runtime@${OCTO_DIGEST} AS octo

FROM alpine:3.22

# ca-certificates so curl and the runtime can verify TLS; tzdata so a named zone
# resolves; curl and jq because that is what the agent was given.
#
# findutils replaces busybox's find with GNU find at the same path. The reason is
# usability rather than security: this image is driven by a language model, and
# busybox's reduced flag surface turns a reasonable -printf into a confusing
# failure.
#
# Be clear that it is NOT a containment choice, because either find admits every
# other program in the image: `find . -maxdepth 0 -exec /bin/sh -c ...` starts a
# shell, and alpine has one. The `allow` list on run_command names the programs
# the model may ASK for; it does not bound what those programs then do. See the
# comment on the run-program flow in config.yaml — the boundary is the pod.
RUN apk add --no-cache ca-certificates curl jq tzdata findutils

# uid 65532, the same non-root user admin-api and admin-web run as, and the same
# one every Octo image uses. The PVC this pod writes to is owned by it.
RUN addgroup -g 65532 -S nonroot \
    && adduser -u 65532 -S -G nonroot -h /home/nonroot nonroot

COPY --from=octo /usr/local/bin/octo /usr/local/bin/octo

# The definition is baked in rather than mounted from a ConfigMap: one artifact,
# one version, nothing to drift between the image the chart names and the flow it
# actually runs. Changing the prompt is a release, which is the right weight for
# changing what an agent is told it may do.
COPY config.yaml /etc/octo/integrations/config.yaml
# The skills, beside it rather than under it: the standalone runtime roots its
# resource loader at the config directory, so `resource: skills/voice.md` in the
# definition resolves from here and nowhere else.
COPY skills /etc/octo/integrations/skills

# Fail the build rather than the pod if a program the definition names is not where
# it says. The allow list is resolved at flow build time, so a wrong path here is a
# crash-loop on first start with a message about a config that "cannot be built" —
# a long way from the line that caused it.
RUN set -eu; \
    for bin in /usr/bin/curl /usr/bin/jq /usr/bin/find; do \
      test -x "$bin" || { echo "missing: $bin"; exit 1; }; \
    done; \
    find --version | head -1 | grep -q GNU

ENV HOME=/home/nonroot

# Created and owned here so the directories exist even on a pod that was given no
# volume: the file connector stats its root when it starts, and a missing directory
# would fail the whole config on load rather than the first message that reached
# for a file.
RUN mkdir -p /data/workspace /data/store && chown -R 65532:65532 /data

USER 65532:65532

ENTRYPOINT ["/usr/local/bin/octo"]
# Load every integration YAML in this directory.
CMD ["run", "--config", "/etc/octo/integrations"]

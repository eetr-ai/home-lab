# The assistant

An [Octo](https://juancavallotti.github.io/octo/) app that answers questions
about this installation, reached from a drawer in the panel.

It is not a service with a flow engine bolted on. `config.yaml` is the whole of
it — one `ai-agent` block, nine tools and six skills — run by Octo's public
standalone runtime. There is no orchestrator and no Octo platform here: one
process, one integration, in-process queues, and a key/value store serialized to
disk.

```text
config.yaml   the definition: connectors, the chat flow, the agent, its tools
skills/       six instruction documents the model loads on demand
Dockerfile    the standalone runtime's binary on a userland it can use
lint.sh       the checks whose failure mode is a crash-loop rather than an error
```

## What it can do

| Tool | |
| --- | --- |
| `admin_read` | `GET` anything from the panel's API, **as the operator who asked** |
| `read_workspace_file`, `write_workspace_file`, `list_workspace` | A directory of its own on the volume |
| `run_command` | `curl`, `jq` and GNU `find`, as argv — there is no shell |
| `recall_facts`, `remember_fact`, `forget_fact` | What it remembers about a person between conversations |
| `navigate_to` | Move the operator's browser to a page in the panel |

It cannot change anything. Every create, drop, scale and restart in this panel is
a screen with a person's hand on it, and the agent's job when asked for one is to
name the page and offer to take you there.

Skills — `home-lab`, `admin-api`, `diagnosing`, `databases`, `commands`, `voice`
— are Markdown documents loaded on demand. Only each one's name and description
sit in the prompt; the body costs nothing until the model asks for it, which is
why there are six of them and a short prompt.

## Three things that are load-bearing

**A `cli-run` allow list is resolved when the flow is *built*.** An entry naming
a program that is not in the image fails the whole config on load, not the first
call that would have used it. That is why the programs are named through
variables (`AGENT_CURL_BIN` and friends) rather than as literals — written as
literals, this definition would stop loading anywhere but the image, including on
a laptop — and why `lint.sh` checks the defaults against the Dockerfile.

**The operator's token arrives as a header.** `X-Operator-Token` lands in the
flow's `vars` and never in `body`, so it cannot reach the model's input, its
memory, or a trace. `lint.sh` asserts the four places it is allowed to appear and
fails on a fifth.

**`allowMethods: [GET]` on `admin_read` is the whole read-only guarantee.** The
API has no per-endpoint authorization of its own — a valid token is a valid token
— so that one line is what stands between a read tool and a write tool. It is a
runtime refusal rather than an instruction: no amount of persuading the model
produces a `POST`. `lint.sh` asserts it, and asserts that nothing else in the file
calls the API connector at all.

## The image

The published runtime image is distroless: one static binary, no shell, no
package manager. So this is that binary on Alpine with `curl`, `jq`,
`findutils` and `ca-certificates` — `juancavallotti/octo-runtime` is a build
stage to copy out of, not a base to add to, which also means there is no Go
toolchain and no compile step in the build.

Octo also publishes an `octo-agenticrunner` that already carries these programs.
It cannot be used: its entrypoint is the k8s-tagged build, which exits without a
runtime services module and wants an orchestrator and a NATS this cluster does
not run.

The definition and the skills are **baked into the image** rather than mounted
from a ConfigMap. One artifact, one version, nothing to drift between the image
the chart names and the flow it runs — and changing the prompt is a release,
which is the right weight for changing what an agent is told it may do.

## Working on it

```bash
task admin-agent:lint     # the definition, against the image that has to run it
task admin-agent:build    # the image
task admin-agent:run      # it, on :8080, against admin/.env
```

`lint.sh` cannot build the flow the way the runtime does — there is no Octo
binary in this repository — so **load the config for real** before you trust a
change. It takes seconds and it is the only thing that catches a bad expression:

```bash
curl -N -X POST localhost:8080/chat \
  -d '{"threadId":"t1","message":"what can you do?"}'
```

Expect `event: agent` frames streaming, then a closing `event: answer`.

One failure worth recognising, because the runtime reports it as a column offset
into a string nobody wrote that way: a YAML `>-` block folds its lines into
spaces only while they share the block's indentation. Indent a continuation line
further and the newline survives — harmless in a CEL expression, a syntax error
inside a quoted literal within one. `lint.sh` checks for it now.

For local development against the panel, `task dev` brings up all three
containers together.

See [admin/.env.example](../.env.example) for what it needs.

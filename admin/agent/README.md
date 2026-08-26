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
| `admin_api` | Call the panel's API, **as the operator who asked** |
| `read_workspace_file`, `write_workspace_file`, `list_workspace` | A directory of its own on the volume |
| `run_command` | `curl`, `jq` and GNU `find`, as argv — there is no shell |
| `recall_facts`, `remember_fact`, `forget_fact` | What it remembers about a person between conversations |
| `navigate_to` | Move the operator's browser to a page in the panel |

Each tool is a `flow-ref` to a sourceless flow rather than an inline chain, which
is what gives it a name a test can call — see [Tests](#tests).

**`admin_api` is not read-only, and that is a decision.** An earlier version
pinned it to `GET`. That withheld half of what the agent is for while withholding
nothing an operator could not already do from the panel's own screens: the API
answers reads through `POST` as well — both query consoles are POSTs — so a verb
is not a proxy for "destructive". The agent calls with the asking operator's own
bearer token, so what it may do is exactly what they may do, and narrowing that
belongs in the token rather than in a definition anyone can edit. What the prompt
and the skills carry instead are the manners: say what you are about to do before
you do it, one change at a time, never guess an identifier for a destructive call,
and offer the page when there is no hurry.

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
memory, or a trace. `lint.sh` asserts the three settings allowed to name it — the
variable that holds it, the check that it is there, and the header it rides — and
fails on a fourth.

**The path guard is the only thing keeping the agent on the panel's own host.**
`admin_api` was a [`rest-dynamic`](https://juancavallotti.github.io/octo/reference/connectors/http)
block, where a connector `baseURL` made leaving impossible and `pathPrefix` was a
runtime refusal. It cannot be: that block refuses a rendered `Authorization`
header, and the connector's own `auth` is configured at startup — so a credential
that differs per operator has nowhere to live. See
[juancavallotti/octo#378](https://github.com/juancavallotti/octo/issues/378).

So the tool is curl, the credential rides on stdin as a curl config file (never
argv, which is readable from the process list and is what a trace records), and
the prefix is a CEL guard that fails closed. That guard is a worse place for a
boundary than a runtime refusal, and it is why `config_test.yaml` gives every way
out of the base URL its own case.

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

## Tests

Two [dolphin](https://juancavallotti.github.io/octo/testing) suites, one per
flow — which is dolphin's unit, and why a flow worth testing gets its own file.

`config_test.yaml` covers `call-admin-api`, the flow behind `admin_api` and the
one carrying the guard that replaced a runtime refusal. Sixteen cases: the
request it builds, the reply it reads, and one per way out of the base URL, each
asserting through a spy `count: 0` that no request was made at all.

`run_program_test.yaml` covers `run-program`, behind `run_command`. Six cases:
which binary each name selects, and the refusals. The refusals are the reason it
exists — the block picks its program with a ternary that tests for curl, then for
jq, and used to let everything else fall through to *find*.

```bash
task admin-agent:test
```

It runs in a container because dolphin needs an Octo runtime and there is no Go
toolchain here to build one. `Dockerfile.test` layers dolphin — copied out of the
published standalone editor image — onto the real agent image, so the suite runs
against the runtime, the definition and the programs that actually ship.
Deliberately a second image: a test runner that drives `octo` is not something the
production image should carry, since `octo` runs whatever definition it is pointed
at.

Some of the cases are there because they already caught something. `cli-run` hands
back stdout with a trailing newline, so splitting it naively leaves an empty last
element — the status reads as `""` and the code stays stuck on the end of the
body. That is a wrong answer rather than a failure, it survived a live end-to-end
check, and four cases fail on it.

## Working on it

```bash
task admin-agent:lint     # the definition, against the image that has to run it
task admin-agent:test     # the dolphin suite
task admin-agent:build    # the image
task admin-agent:run      # it, on :8080, against admin/.env
```

Neither the lint nor the suite builds the whole config the way the runtime does —
dolphin invokes one flow — so **load the config for real** before you trust a
change to anything outside `call-admin-api`. It takes seconds:

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

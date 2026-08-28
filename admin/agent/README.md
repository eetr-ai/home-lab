# The assistant

An [Octo](https://juancavallotti.github.io/octo/) app that answers questions
about this installation, reached from a drawer in the panel.

It is not a service with a flow engine bolted on. `config.yaml` is the whole of
it — one `ai-agent` block, six tools and six skills — run by Octo's public
standalone runtime. There is no orchestrator and no Octo platform here: one
process, one integration, in-process queues, and memory on disk.

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
| `remember`, `forget`, `search_memory` | What it remembers about a person between conversations — the runtime's, not this file's |
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
memory, or a trace. That matters more than it did: conversation history is durable
and never compacted, so a credential reaching it would be written to the volume
rather than lost on the next restart. `lint.sh` asserts the three settings allowed
to name it — the variable that holds it, the check that it is there, and the
`Authorization` header it is rendered into — and fails on a fourth.

**The boundary is the connector's, again.** `admin_api` goes out through a
[`rest-dynamic`](https://juancavallotti.github.io/octo/reference/connectors/http)
block: the connector's `baseURL` fixes the host, and `pathPrefix` and
`allowMethods` refuse on the parsed path before a request is made.

It spent a while as curl instead, because that block used to refuse a rendered
`Authorization` header while a connector's own `auth` is fixed at startup — so a
credential that differs per operator had nowhere to live
([juancavallotti/octo#378](https://github.com/juancavallotti/octo/issues/378)).
The prefix was rebuilt as a CEL guard and the credential handed to curl on stdin
as a config file. octo v0.8.8 lifted the refusal and all of that is gone. What
replaced it is stronger rather than shorter: the checks run on the **parsed**
path, so `%2e%2e` is refused alongside a literal `..` and a `..` inside a query
*value* no longer fails a call that could not have traversed anything.

The connector carries **no `auth`**, deliberately. There is no deployment
credential for this API and there must not be one — a connector credential is
applied only when a request arrives without one, which is exactly the case this
agent must refuse rather than answer as somebody else. `lint.sh` fails if one
appears.

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

`config_test.yaml` covers `call-admin-api`, the flow behind `admin_api`. Eleven
cases: the request it hands the block, the reply it reads back, the API being
unreachable, and the refusal branch — each of the refusals asserting through a spy
`count: 0` that no request was made at all.

It is smaller than it was, and deliberately. When the prefix was a CEL guard,
every way out of the base URL needed a case here, because a clause dropped from an
expression is silent. Now that bounding is the runtime's, and it is not testable
from here — the block is mocked, so its checks never run. The coverage moved in
two directions rather than vanishing: that `pathPrefix`, `allowMethods` and
`failOnError` are **present** is asserted by `lint.sh`, and that they **work** is
octo's own suite.

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

Some of the cases are there because they already caught something. The `run-program`
refusals are the surviving example: the block picked its program with a ternary
that tested for curl, then for jq, and let everything else fall through to *find*,
so `program: "bash"` quietly ran find with the model's arguments.

A whole class of them retired with the curl workaround. `cli-run` hands back stdout
with a trailing newline, so splitting it naively left an empty last element and the
status read as `""` — a wrong answer rather than a failure, which survived a live
end-to-end check. There is no stdout to split any more.

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

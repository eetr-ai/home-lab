# Testing

## Test logic, not structure

A test earns its place by failing when behavior is wrong. A test that asserts a
button says "Delete" fails when someone renames the button, which is not a bug,
and passes when the delete never fires, which is. It costs maintenance forever and
proves nothing, so we do not write it.

**Write tests for:**

- Reducers, and every pure function — draft and dirty-state projections,
  formatters, parsers, the longest-match active-tab rule.
- Go services, table-driven, with the repository substituted by a fake.
- Anything with a rule that is easy to state and easy to get wrong: token refresh,
  identifier quoting, role and privilege composition, pagination boundaries.
- **Every bug, as a reproduction, before the fix.** The test fails first. That is
  the only way to know it tests the thing that was broken.

**Do not write tests for:**

- UI component labels, markup, class names, or the presence of an element. There
  are no DOM tests in this repository and adding a testing-library dependency is a
  decision to be made deliberately, not by accident.
- Getters, constructors, or a function that only forwards to another one.
- Framework behavior. Next.js routing and `pgx` connection pooling are already
  tested by the people who wrote them.

The heuristic: if the test would still pass after you deleted the logic and
returned a constant, it is testing structure.

**Check that an assertion bites before trusting it.** This applies most to the
shell checks under `tests/`, where a `grep` that matches nothing and a `grep` that
matches everything both exit zero in some pipelines. Break the thing on purpose,
watch the check fail, put it back. Several assertions in
`tests/validate-admin-chart.sh` were wrong in exactly that way until they were
mutation-checked.

## No integration tests in the repository

Do not commit a test that needs live infrastructure — a running database, a
cluster, a reachable host. They cannot be automated here, they depend on resources
that are neither disposable nor easily reproduced, and in a public repository they
also document the private environment they need.

Test the logic against a fake instead: a service with a substituted repository
covers the rules, which is where the mistakes are. When the real thing genuinely
needs proving — that a statement the fake accepted is one PostgreSQL also accepts —
run that check by hand, report the result on the pull request, and do not commit
the harness. Writing one temporarily to verify is fine; shipping it is not.

## TDD is the default for logic

For a new service method or a new piece of BFF logic, write the failing test
first. The interface you want falls out of the test, and you find out immediately
whether it is awkward to call.

This is a default, not a ceremony. Wiring, glue, and configuration do not need a
test written first, and sometimes do not need one at all.

## Go

Tests live beside the code as `*_test.go`, in the same package so they can reach
unexported helpers. Prefer table-driven cases with a named `tests` slice; one loop
with ten rows reads better than ten near-identical functions and makes the
difference between cases obvious.

The service's repository interface is what makes this work: supply a struct that
implements it and returns fixed values. No mocking framework.

```bash
task admin:api:test              # all packages
task admin:api:test -- -race     # flags pass through after --
```

Two tests in `internal/openapi` are structural on purpose, and they are the
exception that proves the rule: they compare the routes actually registered
against the routes the OpenAPI description claims. They catch a route added
without an annotation — which regenerates to a byte-identical spec and would
otherwise pass — and a described path that nothing serves. Both are real defects
in the contract the agent depends on.

## TypeScript

Vitest, `environment: "node"`, colocated as `*.test.ts`. Coverage is measured over
`src/lib/**`, which is where the logic is; components are not counted because they
are not tested.

```bash
task admin:web:test
task admin:web:test:watch
```

Keep the thing worth testing out of the component. A draft-comparison rule, a
retry policy, or a URL-derivation rule belongs in its own module, where it can be
tested directly. Extracting it is usually what makes the test worth writing in the
first place.

The agent drawer is the largest example of that in the repository, and it is worth
knowing what it does and does not cover. Four modules under
`src/components/agent/` have no React in them at all — `events.ts` (the SSE
parser), `turns.ts` (the reducer that folds frames into a transcript), `thread.ts`
and `frames.ts` — and those are the ones with tests. The components around them
are not tested, in line with everything above. Two of those tests are load-bearing
rather than routine:

- `parseNavigateEvent` is a **security** boundary, not a formatter. The agent asks
  where to navigate; this decides what it is allowed to ask for, whatever its
  definition was changed to say. Every way out of the site — protocol-relative,
  absolute, backslash, and a path that only becomes protocol-relative once the
  browser drops a tab — has a case.
- The reducer's ordering cases are the feature. A run that thinks, calls two
  tools, thinks again and answers did those things in that order, and a test that
  only counted segments would pass on an implementation that lost it.

## The agent definition

`admin/agent/config.yaml` is not covered by a test, and could not usefully be:
there is no Octo binary in this repository, so nothing here can build the flow the
way the runtime does. `task admin-agent:lint` asserts the properties whose failure
mode is a crash-loop rather than an error somebody would notice — undeclared
variables, a `cli-run` allow list naming a path the image does not carry, a skill
pointing at a file that is not there, `allowMethods` slipping off the read-only
tool, the operator's token reaching somewhere it must not.

None of that is a substitute for **loading the config for real**, which takes
seconds:

```bash
task admin-agent:run
curl -N -X POST localhost:8080/chat -d '{"threadId":"t1","message":"hello"}'
```

Do that before trusting a change to the definition. The lint was written *from*
the failures that found — including a folded YAML block that put a newline inside
a CEL string literal, which the runtime reports as a column offset into a string
nobody wrote that way.

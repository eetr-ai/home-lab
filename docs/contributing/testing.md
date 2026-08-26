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

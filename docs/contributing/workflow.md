# Workflow

## Commits

[Conventional Commits](https://www.conventionalcommits.org/), as
[CONTRIBUTING.md](../../CONTRIBUTING.md) requires. Release automation derives the
version and changelog from them, so the shape is load-bearing rather than
cosmetic:

```text
<type>[optional scope]: <imperative description>
```

Scopes in use: `terraform`, `ansible`, `helm`, `networking`, `databases`, `deps`,
and for this application `admin`, `admin-api`, `admin-web`, `charts`.

Break the work into small, logical commits and **land them as you go**. Each one
should be a coherent slice — one behavior, or one piece of scaffolding — with its
tests in the same commit.

This repository deliberately does **not** require every individual commit to be
independently green and separately approved before the next one starts. It is
simple and single-operator, and that ceremony costs more rework than the
bisectability buys back. What is required is that **the branch is green when the
pull request opens**.

## Pull requests

Branch from the latest `main` with a descriptive name (`feat/admin-panel`,
`fix/nfs-mount`). Fill in the template: intent, verification actually performed,
and operational or rollback impact. Merge once checks pass and review
conversations are resolved.

Run `task check` before opening one. It is the same set CI runs.

## This is a public repository

Never commit private keys, passwords, API tokens, kubeconfig files, Terraform
state, Cloudflare tunnel credentials, database credentials, or unencrypted
Kubernetes Secrets. Environment-specific values live in ignored files with a
tracked `.example` sibling.

The same rule applies to prose: a hostname or address that identifies the private
environment belongs in the private runbook, not in a README.

## Infrastructure expectations

- Prefer declarative and idempotent configuration. A playbook or script must be
  safe to run twice.
- Pin provider, module, action, chart, and image versions. Immutable image tags
  only — never `latest` in a chart.
- Explain destructive or service-interrupting changes in the pull request, and
  include a rollback path.
- Keep the README and the component guides consistent with what is deployed.

## Rules this application inherits

- **Gateway API, never Ingress.** CI fails a rendered chart containing
  `kind: Ingress`.
- **Secrets are pre-created Kubernetes Secrets referenced by name**, never values
  in a chart. The installer verifies each referenced Secret exists before
  installing, so a missing credential fails fast instead of at pod start.
- **A namespace that needs a route must carry
  `home-lab.example/gateway-access=true`**, because the Gateway only admits routes
  from labelled namespaces.
- **Administrative hostnames sit behind Cloudflare Access** before their route is
  enabled.

## Keeping this guidance current

When a design decision or convention is established, write it down here rather
than leaving it in a chat log. Update the closest existing section instead of
starting a parallel copy:

- Layering and code placement → [layer-conventions.md](layer-conventions.md)
- UI, components, theming → [ux-guidelines.md](ux-guidelines.md)
- What to test and what not to → [testing.md](testing.md)
- Commits, pull requests, infrastructure rules → this file

These files are the single source of truth for humans and coding agents alike.
Keep the guidance here, not in tool-specific configuration.

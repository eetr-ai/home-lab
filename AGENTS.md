# Repository guidance

Read these before changing anything. They are the source of truth for coding
standards, layering, testing, and review expectations — keep new decisions in
them rather than in tool-specific configuration.

- [docs/contributing/workflow.md](docs/contributing/workflow.md) — commits, pull
  requests, secret hygiene, and the infrastructure rules that apply to everything
  here.
- [docs/contributing/layer-conventions.md](docs/contributing/layer-conventions.md)
  — how `admin/` is arranged and where new code goes.
- [docs/contributing/testing.md](docs/contributing/testing.md) — what to test, and
  what not to.
- [docs/contributing/ux-guidelines.md](docs/contributing/ux-guidelines.md) — the
  admin panel's components, theme, and interaction contracts.

## What this repository is

A public, reproducible home-lab build: libvirt virtual machines from Terraform, a
kubeadm cluster from Ansible, cluster platform services from Helm, and private
PostgreSQL and MongoDB services running under Docker Compose on the host. The
[README](README.md) is the setup source of truth; environment-specific inventory
lives in a private runbook and never in Git.

`admin/` is the in-cluster panel that manages those databases and reads the
cluster. It is the only application code here.

## Working rules

- **Run `task check` before opening a pull request.** Every check CI runs is
  defined in [Taskfile.yml](Taskfile.yml), so the local and CI answers match.
  `task --list` shows what is available.
- **Break work into small, logical commits and land them as you go.** Use
  [Conventional Commits](https://www.conventionalcommits.org/) — release
  automation depends on them. Individual commits do not need separate approval;
  the branch does need to be green when the pull request opens.
- **This repository is public.** Never commit keys, passwords, tokens, kubeconfig
  files, Terraform state, or unencrypted Kubernetes Secrets. Ignored files get a
  tracked `.example` sibling.
- **Pin versions** for providers, modules, actions, charts, and images. Immutable
  image tags only — never `latest` in a chart.
- **Prefer complete refactors over compatibility shims.** Update every call site
  in the same change; nothing outside this repository depends on these interfaces.

# Contributing

## Workflow

1. Create a branch from the latest `main` using a descriptive name such as
   `feat/libvirt-bootstrap`, `fix/nfs-mount`, or `docs/network-plan`.
2. Keep the change focused and update documentation when behavior or the
   deployed infrastructure changes.
3. Run the relevant validation commands locally.
4. Open a pull request and describe the intent, verification performed, and
   any operational or rollback considerations.
5. Merge only after required checks pass and review conversations are
   resolved. Approval can be required later when the repository has another
   regular maintainer.

## Infrastructure expectations

- Prefer declarative and idempotent configuration.
- Pin provider, module, action, chart, and image versions where practical.
- Do not commit generated state, credentials, or decrypted secrets.
- Explain destructive or service-interrupting changes in the pull request.
- Include a rollback or recovery path for risky changes.
- Keep the README and runbooks consistent with the deployed environment.

## Commit messages

This repository uses [Conventional Commits](https://www.conventionalcommits.org/).
Every commit subject must use the following shape:

```text
<type>[optional scope]: <imperative description>
```

Common types are `feat`, `fix`, `docs`, `chore`, `refactor`, `test`, and `ci`.
Examples:

```text
feat(terraform): provision libvirt kubernetes nodes
fix(cloud-init): load bridge netfilter module
docs(networking): document br0 migration
```

Use `!` and a `BREAKING CHANGE:` footer when a change requires manual migration.

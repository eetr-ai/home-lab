# Contributing guides

The conventions for changing this repository, and the source of truth for both
human contributors and coding agents. When a decision is made, it is written down
here rather than left in a chat log.

| Guide | Covers |
| --- | --- |
| [workflow.md](workflow.md) | Commits, pull requests, secret hygiene, and the infrastructure rules the application inherits |
| [layer-conventions.md](layer-conventions.md) | How `admin/` is arranged — components folded by feature, the three layers inside each, and where new code goes |
| [testing.md](testing.md) | What to test and, just as importantly, what not to |
| [ux-guidelines.md](ux-guidelines.md) | The admin panel's component library, theme tokens, directory-surface contract, and overlay rules |

Operational documentation lives one level up in [`docs/`](../), and each component
keeps its own guide beside its code — [`ansible/`](../../ansible/README.md),
[`terraform/`](../../terraform/README.md),
[`charts/platform/`](../../charts/platform/README.md), and
[`databases/`](../../databases/README.md).

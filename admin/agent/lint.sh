#!/usr/bin/env bash

# Check the agent definition against the image that has to run it.
#
# There is no Octo binary in this repository, so this cannot build the flow the way
# the runtime does. What it can do is assert the handful of properties whose
# failure mode is a crash-loop rather than a compile error — every one of these has
# a symptom that shows up a long way from its cause:
#
#   - A `${VAR}` with no declaration substitutes to nothing, and the failure is
#     wherever the empty string lands.
#   - A `cli-run` allow list is resolved when the flow is BUILT, so a literal path
#     that is not in the image fails the whole config on load, and a path that no
#     longer matches the Dockerfile fails the same way after an innocent bump.
#   - `allowMethods: [GET]` on admin_read is the entire read-only guarantee. The
#     admin API has no per-endpoint authorization of its own, so losing that line
#     turns a read tool into a write tool with nothing else to notice.
#
# PyYAML rather than a hand-rolled parser: `task lint` already requires
# ansible-core, which depends on it.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

python3 - config.yaml Dockerfile <<'PY'
import re
import sys
from pathlib import Path

import yaml

config_path, dockerfile_path = sys.argv[1], sys.argv[2]
HERE = Path(config_path).resolve().parent

with open(config_path, encoding="utf-8") as handle:
    raw = handle.read()
config = yaml.safe_load(raw)

with open(dockerfile_path, encoding="utf-8") as handle:
    dockerfile = handle.read()

problems = []

# --- every ${VAR} is declared -------------------------------------------------
declared = {entry["name"] for entry in config.get("env", [])}
# Two spellings, and both are real: ${NAME} is substituted into the parsed YAML
# before it is decoded, while env.NAME is read by a CEL expression at run time.
referenced = set(re.findall(r"\$\{([A-Z0-9_]+)\}", raw))
referenced |= set(re.findall(r"\benv\.([A-Z0-9_]+)\b", raw))
for name in sorted(referenced - declared):
    problems.append(f"${{{name}}} is used but not declared under env:")

# A declaration nothing reads is not an error the runtime will ever report, but it
# is always either a rename that was only half done or a setting that stopped
# working silently.
for name in sorted(declared - referenced):
    problems.append(f"env: declares {name}, which nothing references")


def walk(node):
    """Yield every mapping in the tree, blocks and settings alike."""
    if isinstance(node, dict):
        yield node
        for value in node.values():
            yield from walk(value)
    elif isinstance(node, list):
        for value in node:
            yield from walk(value)


blocks = list(walk(config))

# --- allow lists name variables, never literal paths --------------------------
#
# The defaults are the image's layout; writing one as a literal here would make the
# definition unloadable anywhere else, including on a laptop, and wrapping the block
# in an `if` would not help because the builder builds both branches.
bin_defaults = {
    entry["name"]: str(entry.get("default", ""))
    for entry in config.get("env", [])
    if entry["name"].endswith("_BIN")
}
allow_entries = [entry for block in blocks for entry in block.get("allow", [])]
if not allow_entries:
    problems.append("no cli-run allow list found; this check would pass on anything")
for entry in allow_entries:
    match = re.fullmatch(r"\$\{([A-Z0-9_]+)\}", str(entry))
    if match is None:
        problems.append(f"allow list entry {entry!r} is a literal, not a ${{VAR}}")
    elif match.group(1) not in bin_defaults:
        problems.append(f"allow list entry {entry!r} does not name a *_BIN variable")

# --- ...and the image carries what those variables point at -------------------
for name, path in sorted(bin_defaults.items()):
    if path not in dockerfile:
        problems.append(f"{name} defaults to {path}, which the Dockerfile never verifies")

# --- admin_read is read-only --------------------------------------------------
agent = next(
    block for block in blocks if block.get("type") == "ai-agent"
)
tools = {tool["name"]: tool for tool in agent["tools"]}

admin_calls = [
    block
    for block in walk(tools["admin_read"])
    if block.get("type") == "rest-dynamic"
]
if len(admin_calls) != 1:
    problems.append(f"admin_read makes {len(admin_calls)} dynamic calls; expected exactly 1")
for call in admin_calls:
    settings = call.get("settings", {})
    if settings.get("allowMethods") != ["GET"]:
        problems.append("admin_read must set allowMethods: [GET] — it is the only read-only guarantee")
    if settings.get("pathPrefix") != "/api":
        problems.append("admin_read must set pathPrefix: /api")

# ...and nothing else in the file may call the admin connector at all. Without this
# a second, unrestricted rest-dynamic could be added and admin_read would still pass.
for block in blocks:
    if block.get("type") not in {"rest-dynamic", "rest"}:
        continue
    if block.get("settings", {}).get("connector") != "admin":
        continue
    if block not in admin_calls:
        problems.append("a block outside admin_read calls the admin connector")

# --- the operator's token never reaches the model -----------------------------
#
# It arrives as a header so that it lands in vars rather than in body. The one
# place it may be named is the Authorization header admin_read sends; anywhere
# else — the agent's input, a payload, a log line — puts a live credential into a
# transcript, a memory object and a trace.
# Exactly the four places it may appear, spelled out. Anything else — the agent's
# input, a payload, a log line — puts a live credential into a transcript, a
# memory object and a trace.
allowed_token_uses = {
    ("name", "operatorToken"),
    ("condition", 'vars.operatorToken != ""'),
    ("headers", '{"Authorization": "Bearer " + vars.operatorToken}'),
}
seen_token_uses = set()
for block in blocks:
    for key, value in block.items():
        if not isinstance(value, str) or "operatorToken" not in value:
            continue
        use = (key, value.strip())
        if use not in allowed_token_uses:
            problems.append(f"operatorToken reaches {key}: {value.strip()[:70]}")
        seen_token_uses.add(use)

# Both directions: an allowed use that has vanished means the wiring was removed
# and this list is now describing something that is not there.
for key, value in sorted(allowed_token_uses - seen_token_uses):
    problems.append(f"the operator token no longer reaches {key}; the wiring changed")

# ...and it has to arrive as a header rather than in the body, which is what keeps
# it out of the agent's input in the first place.
source_settings = config["flows"][0]["source"]["settings"]
if "X-Operator-Token" not in source_settings.get("headers", []):
    problems.append("the chat source does not expose X-Operator-Token, so vars cannot hold it")

holder = next(
    (
        block.get("settings", {})
        for block in blocks
        if block.get("type") == "set-variable"
        and block.get("settings", {}).get("name") == "operatorToken"
    ),
    None,
)
if holder is None:
    problems.append("nothing sets the operatorToken variable")
elif 'vars["X-Operator-Token"]' not in holder.get("value", ""):
    problems.append("operatorToken is not read from the X-Operator-Token header")

# --- no newline inside a CEL string literal -----------------------------------
#
# The one that actually bit. A YAML `>-` block folds its lines into spaces only
# while they share the block's indentation; indent a continuation line further and
# YAML keeps the newline. Inside a CEL expression that is harmless — newlines are
# whitespace — but inside a quoted literal within one it is a syntax error, and the
# runtime reports it as a column offset into a string nobody wrote that way.
CEL_KEYS = {
    "value", "condition", "path", "key", "args", "program", "stdin", "headers",
    "query", "method", "subject", "content", "default", "input", "stopWhen",
    "memoryThreadId", "data", "resultVar", "as", "existsVar",
}


def newline_in_literal(expression):
    quote = None
    for char in expression:
        if quote is None and char in "\"'":
            quote = char
        elif quote is not None and char == quote:
            quote = None
        elif quote is not None and char == "\n":
            return True
    return False


for block in blocks:
    for key, value in block.items():
        if key not in CEL_KEYS or not isinstance(value, str) or "\n" not in value:
            continue
        if newline_in_literal(value):
            problems.append(
                f"{key}: a string literal spans a line break — "
                "de-indent the folded block so YAML joins the lines"
            )

# --- every skill has a document, and every document a skill --------------------
#
# A skill naming an alias that resources does not declare, or an alias pointing at
# a file that is not there, fails when the model first asks for it — a long way
# from the change that broke it, and only on the turn that needed it.
aliases = {
    template["as"]: template["resource"]
    for template in config.get("resources", {}).get("templates", [])
}
skills = {skill["name"]: skill for skill in agent.get("skills", [])}
if not skills:
    problems.append("the agent declares no skills; this check would pass on anything")

for name, skill in skills.items():
    alias = skill.get("resource")
    if alias not in aliases:
        problems.append(f"skill {name} names resource {alias!r}, which resources.templates does not declare")
    elif not (HERE / aliases[alias]).is_file():
        problems.append(f"skill {name} points at {aliases[alias]}, which is not in this directory")
    if not skill.get("description", "").strip():
        problems.append(f"skill {name} has no description; it is all the model sees up front")

for alias in sorted(set(aliases) - {skill.get("resource") for skill in skills.values()}):
    problems.append(f"resources declares {alias!r}, which no skill loads")

# load_skill is reserved — a tool may not take the name, and a skill may not share
# one with a tool. Both would load and then behave in a way nothing explains.
if "load_skill" in tools:
    problems.append("load_skill is reserved and cannot be a tool name")
for name in sorted(set(skills) & set(tools)):
    problems.append(f"{name} is both a skill and a tool")

# --- every tool is described --------------------------------------------------
for name, tool in tools.items():
    if not tool.get("description", "").strip():
        problems.append(f"tool {name} has no description")
    if "inputSchema" not in tool:
        problems.append(f"tool {name} has no inputSchema; the model gets an open object")
    if not tool.get("process"):
        problems.append(f"tool {name} has no process chain")

if problems:
    print("The agent definition has problems:", file=sys.stderr)
    for problem in problems:
        print(f"  - {problem}", file=sys.stderr)
    raise SystemExit(1)

print(
    f"Agent definition validated: {len(tools)} tools, {len(skills)} skills, "
    f"{len(declared)} variables."
)
PY

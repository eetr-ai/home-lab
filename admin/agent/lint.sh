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
#   - The path guard on admin_api is the only thing keeping a model-supplied
#     string under the panel's own base URL, now that the connector that used to
#     enforce it is out of the picture. Losing a clause is a hole, not a lint nit.
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

# --- every connector is used --------------------------------------------------
#
# Connectors start eagerly, so a dead one is a startup cost and a standing claim
# that something uses it. The admin API's http-client connector became dead the
# moment its tool moved to curl, and nothing in the runtime would ever have said so.
declared_connectors = {c["name"] for c in config.get("connectors", [])}
used_connectors = {
    block["settings"]["connector"]
    for block in blocks
    if isinstance(block.get("settings"), dict) and "connector" in block["settings"]
}
used_connectors |= {
    flow["source"]["connector"]
    for flow in config["flows"]
    if isinstance(flow.get("source"), dict) and "connector" in flow["source"]
}
# The agent block names its LLM connector directly rather than through settings.
used_connectors |= {
    block["connector"] for block in blocks
    if block.get("type") == "ai-agent" and "connector" in block
}
for name in sorted(declared_connectors - used_connectors):
    problems.append(f"connector {name!r} is declared but nothing uses it")

# --- admin_api is bounded to the panel's own base URL ------------------------
#
# This block carries more weight than it looks like it should, and the reason is
# worth stating: admin_api used to be a `rest-dynamic` block, where the path prefix
# was a runtime refusal. It cannot be, because that block refuses a rendered
# Authorization header and the connector's own auth is configured at startup — see
# juancavallotti/octo#378. So that boundary is now a guard in this file, and this
# is what holds it there.
#
# The METHOD is deliberately not restricted, and there is no assertion about it on
# purpose. That was a decision: a verb is not a proxy for "destructive" here, the
# agent calls with the asking operator's own token, and narrowing what may be done
# belongs in the token rather than in a definition anyone can edit.
agent = next(block for block in blocks if block.get("type") == "ai-agent")
tools = {tool["name"]: tool for tool in agent["tools"]}
by_name = {flow["name"]: flow for flow in config["flows"]}


def implementation(tool_name):
    """The blocks of the flow a tool delegates to, following its flow-ref.

    Every tool is a flow-ref to a sourceless flow, so an assertion about what a
    tool does has to follow that hop — which also means a tool pointed at the
    wrong flow, or at one that does not exist, fails here rather than at the
    first call.
    """
    tool = tools.get(tool_name)
    if tool is None:
        problems.append(f"there is no {tool_name} tool")
        return []
    refs = [b for b in walk(tool) if b.get("type") == "flow-ref"]
    if len(refs) != 1:
        problems.append(f"{tool_name} should be exactly one flow-ref; it has {len(refs)}")
        return []
    target = refs[0].get("settings", {}).get("flow")
    if target not in by_name:
        problems.append(f"{tool_name} delegates to flow {target!r}, which does not exist")
        return []
    if "source" in by_name[target]:
        problems.append(f"flow {target!r} has a source, so flow-ref cannot call it")
    return list(walk(by_name[target]))


# Every tool delegates, and every delegate resolves. Checked for all of them and
# not only the ones asserted below, so a tool wired to the wrong flow is caught.
for tool_name in tools:
    implementation(tool_name)

# ...and no sourceless flow is left behind unused, which is what a rename looks
# like when only half of it happened.
referenced = {
    b.get("settings", {}).get("flow")
    for b in blocks
    if b.get("type") == "flow-ref"
}
for flow in config["flows"]:
    if "source" not in flow and flow["name"] not in referenced:
        problems.append(f"flow {flow['name']!r} has no source and nothing calls it")

admin_blocks = implementation("admin_api")

# Nothing may call an HTTP connector any more. If a rest or rest-dynamic block
# comes back — because the upstream issue was fixed, say — every assertion below
# stops describing the thing that runs, and silently.
for block in blocks:
    if block.get("type") in {"rest", "rest-dynamic"}:
        problems.append(
            "a rest/rest-dynamic block is back; the assertions here are about the "
            "curl workaround and need rewriting for it"
        )

calls = [
    block for block in admin_blocks
    if block.get("type") == "cli-run" and block.get("program") == "env.AGENT_CURL_BIN"
]
if len(calls) != 1:
    problems.append(f"admin_api makes {len(calls)} curl calls; expected exactly 1")

for call in calls:
    stdin = call.get("stdin", "")
    argv = call.get("args", "")

    # The credential and the request body go on stdin and never in argv: a
    # process's arguments are readable from the process list, and argv is what a
    # trace records. The body matters as much as the token here — creating a
    # database role puts a password in it.
    for name, value in [("the operator token", "operatorToken"), ("the request body", "requestBody")]:
        if value in argv:
            problems.append(f"{name} must not appear in curl's arguments")
    if "Authorization: Bearer " not in stdin:
        problems.append("admin_api does not send the operator's bearer token")
    if '"-K", "-"' not in argv:
        problems.append("admin_api must pass -K - so curl reads the credential from stdin")

    # The method is the model's to choose, but only from the enum its schema
    # declares — so the schema is what bounds it, and an unconstrained one would
    # let a rendered string reach curl's config as a config directive.
    schema = tools["admin_api"].get("inputSchema", "")
    if '"enum": ["GET", "POST", "PUT", "PATCH", "DELETE"]' not in schema:
        problems.append("admin_api's method must be an enum in the schema; it is what bounds what reaches curl's config")

# What pathPrefix used to do. Each clause closes a way out of the base URL, so a
# missing one is a hole rather than a lint nit; they are listed rather than
# summarised so removing one cannot pass.
guard = next(
    (
        block.get("settings", {})
        for block in admin_blocks
        if block.get("type") == "set-variable"
        and block.get("settings", {}).get("name") == "pathOk"
    ),
    None,
)
if guard is None:
    problems.append("admin_api has no path guard; a model-supplied path would reach the URL unchecked")
else:
    expression = guard.get("value", "")
    # Every clause, spelled out as it appears rather than by a keyword that would
    # match a different expression. The list was short by one — the backslash
    # clause — and "matches" alone would have been satisfied by any regex at all,
    # which is the sort of assertion that reads as coverage and is not.
    for clause, why in [
        ('body.path.startsWith("/api")', "the path prefix"),
        ('!body.path.startsWith("//")', "a protocol-relative path reaching another host"),
        ('!body.path.contains("..")', "a .. segment escaping the prefix"),
        ('!body.path.contains("\\\\")', "a backslash some parsers read as a slash"),
        ('!body.path.contains(":")', "a scheme"),
        ('!body.path.matches("\\\\s")', "whitespace splitting the URL into another argument"),
    ]:
        if clause not in expression:
            problems.append(f"admin_api's path guard no longer refuses {why}")

# ...and both halves have to gate the call, not just the token.
gate = next(
    (block for block in admin_blocks if block.get("type") == "if"),
    None,
)
if gate is None:
    problems.append("admin_api does not gate the call at all")
else:
    condition = gate.get("condition", "")
    if "vars.pathOk" not in condition or 'vars.operatorToken != ""' not in condition:
        problems.append("admin_api must call only with a token AND a path that passed the guard")

# --- the operator's token never reaches the model -----------------------------
#
# It arrives as a header so that it lands in vars rather than in body. Anywhere it
# is named other than the three places below — the agent's input, a payload, a log
# line, curl's argv — puts a live credential into a transcript, a memory object and
# a trace.
#
# `vars.operatorToken` is the RAW token, and it is exhaustively listed. What the
# request actually carries is `vars.curlToken`, the escaped form. Keeping the two
# names apart is what makes this checkable at all: an earlier version of this block
# exempted the whole stdin expression on the strength of it containing the word
# Authorization, which let the raw token be substituted back into the header with
# nothing noticing. It was a mutation test that found that, not review.
allowed_token_uses = {
    ("name", "operatorToken"),
    ("condition", 'vars.operatorToken != "" && vars.pathOk'),
    # Escaped for curl's config format, exactly as the request body is. The raw
    # token goes no further than this line; what reaches the header is curlToken.
    ("value", r'vars.operatorToken.replace("\\", "\\\\").replace("\"", "\\\"")'),
}
seen_token_uses = set()
for block in blocks:
    for key, value in block.items():
        if not isinstance(value, str) or "operatorToken" not in value:
            continue
        use = (key, value.strip())
        if use in allowed_token_uses:
            seen_token_uses.add(use)
            continue
        # The else branch's presence check, which reads the variable without
        # rendering it anywhere.
        if key == "value" and 'vars.operatorToken == ""' in value:
            continue
        problems.append(f"operatorToken reaches {key}: {value.strip()[:70]}")

# ...and the request carries the escaped form, from the one block that builds it.
for call in calls:
    stdin = call.get("stdin", "")
    if "vars.curlToken" not in stdin:
        problems.append("curl's config does not send the escaped token")
    if "operatorToken" in stdin:
        problems.append(
            "curl's config names the raw operatorToken; it must send vars.curlToken, "
            "the form escaped for the config file"
        )

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

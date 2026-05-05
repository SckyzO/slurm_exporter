#!/usr/bin/env python3
"""
Add a "Slurm Dashboards" dropdown link to each dashboard so users can jump
between dashboards from the top bar without going back to the home menu.

The link uses Grafana's `type: "dashboards"` + `asDropdown: true` schema:
Grafana auto-populates the dropdown with every dashboard that has the
matching `tags`. All 10 slurm_exporter dashboards are tagged "slurm",
so a single tag filter is enough.

Idempotent: re-running on a dashboard that already has the entry is a
no-op. Existing entries in the `links` array (e.g. the GitHub link) are
preserved.

Requires Grafana 12+ (panel/dashboard JSON schema this script targets).

Usage:
    python3 scripts/dashboards_add_drilldown_links.py
"""
import glob
import json
import re
import sys

DRILLDOWN_LINK = {
    "asDropdown": True,
    "icon": "external link",
    "includeVars": True,
    "keepTime": True,
    "tags": ["slurm"],
    "targetBlank": False,
    "title": "Slurm Dashboards",
    "tooltip": "Switch to another Slurm dashboard",
    "type": "dashboards",
    "url": "",
}

# Marker used to detect "already added" — we look for the title in the file.
MARKER = '"title": "Slurm Dashboards"'

# Match the opening of the links array, with the first child object opening.
# Captures everything up to (but not including) the first "{" of the first
# existing entry, so we can insert our new entry before it.
LINKS_OPEN = re.compile(
    r'("links"\s*:\s*\[\s*\n)(\s*)\{', re.MULTILINE
)


def render_block(indent: str) -> str:
    """Render the new link as a JSON object indented at the same level
    as the existing entries."""
    text = json.dumps(DRILLDOWN_LINK, indent=2, ensure_ascii=False)
    # text is "{\n  \"asDropdown\": true,\n  ...\n}"
    # Re-indent every line by `indent` (the indent of existing array entries).
    lines = text.split("\n")
    indented = [indent + lines[0]] + [indent + line for line in lines[1:]]
    return "\n".join(indented)


def fix_file(path: str) -> bool:
    with open(path, encoding="utf-8") as f:
        text = f.read()

    if MARKER in text:
        return False

    m = LINKS_OPEN.search(text)
    if not m:
        # No existing links array with at least one entry: skip silently.
        # (We could create the array from scratch but every dashboard
        # already has at least the GitHub link, so we don't need to.)
        raise RuntimeError(f"{path}: no `links: [ {{...}} ]` pattern found")

    indent = m.group(2)  # spaces before the existing `{`
    new_block = render_block(indent)

    # Replace "<links_open>{indent}{" with "<links_open>{new_block},\n{indent}{"
    new_text = (
        text[: m.end(1)]
        + new_block
        + ",\n"
        + text[m.end(1):]
    )

    # Sanity: re-parse.
    json.loads(new_text)

    with open(path, "w", encoding="utf-8") as f:
        f.write(new_text)
    return True


def main() -> int:
    paths = sorted(glob.glob("dashboards_grafana/*.json"))
    if not paths:
        print("No dashboards found in dashboards_grafana/", file=sys.stderr)
        return 1
    changed = 0
    for p in paths:
        if fix_file(p):
            print(f"  + {p}")
            changed += 1
        else:
            print(f"  = {p} (already has Slurm Dashboards dropdown)")
    print(f"\n{changed} file(s) updated, {len(paths) - changed} unchanged.")
    return 0


if __name__ == "__main__":
    sys.exit(main())

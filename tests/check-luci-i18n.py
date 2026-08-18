#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later

import ast
import json
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
LUCI_ROOT = ROOT / "luci-app-steer"
PO_PATH = LUCI_ROOT / "po" / "zh_Hans" / "steer.po"
MENU_PATH = LUCI_ROOT / "root" / "usr" / "share" / "luci" / "menu.d" / "luci-app-steer.json"


def fail(message: str) -> None:
    print(f"check-luci-i18n: {message}", file=sys.stderr)
    raise SystemExit(1)


menu = json.loads(MENU_PATH.read_text())
source_ids = {entry["title"] for entry in menu.values() if "title" in entry}
for path in (LUCI_ROOT / "htdocs").rglob("*.js"):
    source_ids.update(re.findall(r"_\('([^']*)'\)", path.read_text()))

translations = {}
current_id = None
for line_number, line in enumerate(PO_PATH.read_text().splitlines(), 1):
    if line.startswith("msgid "):
        current_id = ast.literal_eval(line[6:])
    elif line.startswith("msgstr ") and current_id:
        if current_id in translations:
            fail(f"duplicate msgid {current_id!r} at line {line_number}")
        translations[current_id] = ast.literal_eval(line[7:])
        current_id = None

missing = sorted(source_ids - translations.keys())
untranslated = sorted(key for key in source_ids if not translations.get(key))
obsolete = sorted(translations.keys() - source_ids)
bad_placeholders = sorted(
    key
    for key in source_ids
    if re.findall(r"%(?:\d+\$)?[a-zA-Z%]", key)
    != re.findall(r"%(?:\d+\$)?[a-zA-Z%]", translations.get(key, ""))
)

if missing:
    fail("missing translations:\n" + "\n".join(missing))
if untranslated:
    fail("empty translations:\n" + "\n".join(untranslated))
if obsolete:
    fail("obsolete translations:\n" + "\n".join(obsolete))
if bad_placeholders:
    fail("placeholder mismatch:\n" + "\n".join(bad_placeholders))

print(f"LuCI Simplified Chinese coverage: {len(source_ids)}/{len(source_ids)}")

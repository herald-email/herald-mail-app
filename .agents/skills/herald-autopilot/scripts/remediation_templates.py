#!/usr/bin/env python3
from __future__ import annotations

import json
from pathlib import Path


def templates_path(repo_root: Path) -> Path:
    return repo_root / ".agents" / "skills" / "herald-autopilot" / "references" / "remediation-templates.json"


def load_remediation_templates(repo_root: Path) -> dict[str, dict]:
    path = templates_path(repo_root)
    if not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def match_remediation_template(name: str, templates: dict[str, dict]) -> tuple[str | None, dict | None]:
    if not name:
        return None, None

    normalized = name.strip().lower()
    for key, template in templates.items():
        aliases = [alias.lower() for alias in template.get("aliases", [])]
        if normalized == key.lower() or normalized in aliases:
            return key, template

    for key, template in templates.items():
        aliases = [alias.lower() for alias in template.get("aliases", [])]
        if key.lower() in normalized:
            return key, template
        for alias in aliases:
            if alias and alias in normalized:
                return key, template

    return None, None

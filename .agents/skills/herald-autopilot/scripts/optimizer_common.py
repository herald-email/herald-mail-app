#!/usr/bin/env python3
from __future__ import annotations

import datetime as dt
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from artifact_io import load_json, save_json, save_text


@dataclass
class RunRecord:
    run_dir: Path
    run: dict[str, Any]
    score: dict[str, Any] | None

    @property
    def run_id(self) -> str:
        return self.run["run_id"]

    @property
    def created_at(self) -> str:
        return self.run.get("created_at", "")


def now_utc() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()


def state_dir(repo_root: Path) -> Path:
    return repo_root / ".superpowers" / "autopilot" / "state"


def runs_root(repo_root: Path) -> Path:
    return repo_root / ".superpowers" / "autopilot" / "runs"


def list_runs(repo_root: Path, limit: int | None = None) -> list[RunRecord]:
    records: list[RunRecord] = []
    root = runs_root(repo_root)
    if not root.exists():
        return records

    for run_dir in sorted(root.iterdir()):
        run_path = run_dir / "run.json"
        if not run_path.exists():
            continue
        run = load_json(run_path)
        score_path = run_dir / "score.json"
        score = load_json(score_path) if score_path.exists() else None
        records.append(RunRecord(run_dir=run_dir, run=run, score=score))

    records.sort(key=lambda item: item.created_at, reverse=True)
    if limit is not None:
        return records[:limit]
    return records

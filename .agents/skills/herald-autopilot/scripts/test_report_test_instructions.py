from __future__ import annotations

import json
import subprocess
import tempfile
import unittest
from pathlib import Path


SCRIPT_DIR = Path(__file__).resolve().parent
BOOTSTRAP = SCRIPT_DIR / "bootstrap_run.py"
RENDER = SCRIPT_DIR / "render_report.py"


def run_python(script: Path, *args: str, cwd: Path) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["python3", str(script), *args],
        cwd=cwd,
        check=True,
        capture_output=True,
        text=True,
    )


def load_json(path: Path) -> dict:
    return json.loads(path.read_text())


class ReportTestInstructionsTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.repo_root = Path(self.temp_dir.name)

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def bootstrap_run(self) -> Path:
        result = run_python(
            BOOTSTRAP,
            "--repo-root",
            str(self.repo_root),
            "--task",
            "Fix a user-visible Herald workflow that touches multiple surfaces",
            "--task-type",
            "bug",
            "--surfaces",
            "code,tui,mcp,ssh",
            "--plan-summary",
            "Verify the changed behavior with focused checks and surface smoke tests.",
            cwd=self.repo_root,
        )
        return Path(result.stdout.strip())

    def test_report_includes_copy_paste_test_instructions(self) -> None:
        run_dir = self.bootstrap_run()
        run = load_json(run_dir / "run.json")
        worktree = Path(run["paths"]["worktree"])
        worktree.mkdir(parents=True)

        run_python(RENDER, "--run-dir", str(run_dir), cwd=self.repo_root)

        report = Path(run["paths"]["report_path"]).read_text()
        self.assertIn("## How To Test This Change", report)
        self.assertIn("Candidate binary:", report)
        self.assertIn(f"{worktree}/bin/herald --demo", report)
        self.assertIn("go test ./...", report)
        self.assertIn("tmux new-session -d -s herald-test", report)
        self.assertIn("./bin/herald-mcp-server --demo", report)
        self.assertIn("go build -o ./bin/herald-ssh-server ./cmd/herald-ssh-server", report)


if __name__ == "__main__":
    unittest.main()

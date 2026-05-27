from __future__ import annotations

import unittest
from pathlib import Path

from remediation_templates import load_remediation_templates, match_remediation_template


SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parents[3]


class RemediationTemplateTests(unittest.TestCase):
    def test_user_repro_alias_matches_post_handoff_template(self) -> None:
        templates = load_remediation_templates(REPO_ROOT)

        key, template = match_remediation_template("user-repro-after-ed02a1d", templates)

        self.assertEqual(key, "user-repro-after-commit")
        self.assertIsNotNone(template)
        self.assertIn("User-reproduced post-handoff failure", template["title"])
        self.assertTrue(any("exact user repro command" in item for item in template["checklist"]))

    def test_red_compose_comma_alias_still_matches_input_routing_template(self) -> None:
        templates = load_remediation_templates(REPO_ROOT)

        key, template = match_remediation_template("template-red-compose-comma-alias-feedback", templates)

        self.assertEqual(key, "input-routing-safety")
        self.assertIsNotNone(template)
        self.assertIn("text-entry surface", " ".join(template["checklist"]))

    def test_green_demo_key_overlay_alias_matches_demo_overlay_template(self) -> None:
        templates = load_remediation_templates(REPO_ROOT)

        key, template = match_remediation_template("template-green-demo-key-overlay-app-attempt1-feedback", templates)

        self.assertEqual(key, "demo-key-overlay")
        self.assertIsNotNone(template)
        checklist = " ".join(template["checklist"])
        self.assertIn("import", checklist)
        self.assertIn("--demo --demo-keys", checklist)
        self.assertIn("text-entry", checklist)

    def test_user_review_followup_settings_hints_alias_matches_template(self) -> None:
        templates = load_remediation_templates(REPO_ROOT)

        key, template = match_remediation_template("template-user-review-followup-settings-hints-feedback", templates)

        self.assertEqual(key, "user-review-followup-settings-hints")
        self.assertIsNotNone(template)
        checklist = " ".join(template["checklist"])
        self.assertIn("user follow-up", checklist)
        self.assertIn("settings", checklist)
        self.assertIn("bottom hints", checklist)

    def test_commit_hook_make_test_alias_matches_template(self) -> None:
        templates = load_remediation_templates(REPO_ROOT)

        key, template = match_remediation_template("commit hook make test", templates)

        self.assertEqual(key, "commit-hook-make-test")
        self.assertIsNotNone(template)
        checklist = " ".join(template["checklist"])
        self.assertIn("pre-commit", checklist)
        self.assertIn("PATH", checklist)
        self.assertIn("local loopback", checklist)


if __name__ == "__main__":
    unittest.main()

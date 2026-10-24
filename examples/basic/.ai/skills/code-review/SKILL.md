---
name: code-review
description: Standard code review checklist and workflow for pull requests
---
# Code Review Skill

This skill guides agents and contributors through standard pull request reviews.

## Review Workflow
1. Verify functional correctness against the issue description or requirements.
2. Confirm error handling, edge cases, and resource cleanup (e.g., deferring close calls).
3. Check test coverage and verify that new tests fail before the fix/feature and pass after.
4. Consult `references/checklist.md` for specific criteria before approving changes.

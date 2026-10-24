---
name: code-review
description: Comprehensive code review checklist, review scripts, and references
---
# Code Review Skill

This skill provides automated checks and review checklists for pull requests.

## Workflow
1. Run `./scripts/check.sh` locally to verify formatting and basic lint rules.
2. Review the criteria in `./references/checklist.md`.
3. Verify tests pass with `go test -v ./...`.

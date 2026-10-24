---
name: testing
description: Testing procedures, conventions, and example test implementations
---
# Testing Skill

Guidelines for writing and organizing automated tests across this project.

## Conventions
- Use table-driven tests with standard fields (`name`, `input`, `want`, `wantErr`).
- Subtests must be executed using `t.Run(tc.name, func(t *testing.T) { ... })`.
- See `examples/sample-test.md` for a complete reference implementation.

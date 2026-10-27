---
name: testing
description: Guidelines and procedures for executing test suites
---
# Testing Skill

Use this skill when implementing or verifying tests across the project.

## Instructions
- Run unit tests: `go test -v ./...`
- Run tests with race detection: `go test -race -v ./...`
- Verify benchmark performance: `go test -bench=. ./...`

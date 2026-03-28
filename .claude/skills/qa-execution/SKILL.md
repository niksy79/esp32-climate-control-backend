---
name: qa-execution
description: Standardizes QA execution output for all test runs.
---

For every test case, output exactly:

## Test Case
ID:

## Preconditions
-

## Steps Executed
1.

## Expected
-

## Actual
-

## Status
PASS / FAIL

## Evidence
- response:
- screenshot:
- logs:
- mqtt/ws capture:

Rules:
- never skip expected vs actual
- never mark PASS without evidence
- be concise and factual
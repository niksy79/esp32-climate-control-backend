---
name: bug-reporting
description: Creates structured bug reports from failed QA cases.
---

Format every bug as:

## Bug Title
## Severity
## Area
## Related Test Case
## Preconditions
## Steps to Reproduce
## Expected Result
## Actual Result
## Evidence
## Suspected Scope
## Regression Tests Needed

Severity rules:
- Critical: blocks core usage, auth, tenant isolation, live telemetry, or major admin operations
- Major: feature broken but workaround exists
- Minor: UI inconsistency, low-risk defect, copy issue
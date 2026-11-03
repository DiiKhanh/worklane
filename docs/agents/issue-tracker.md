# Issue tracker: GitHub

Issues and new ticket-based specs live in GitHub Issues:
https://github.com/DiiKhanh/worklane/issues

Existing specs and plans under docs/superpowers/ remain reference material.

## Operations

- Use gh-axi for supported GitHub operations. Consult its command help.
  Use gh only when the wrapper lacks the required capability.
- Verify the target repository is DiiKhanh/worklane before writes.
- Publishing to the issue tracker means creating a GitHub issue.
- Fetching a relevant ticket means reading its body, labels and comments.
- For multiline bodies, use a file and --body-file where supported.
- Use native GitHub sub-issues for parent-child relationships and native
  issue dependencies for blockers. If unavailable, record "Part of #N"
  and "Blocked by: #N" in the child issue body.
- A ticket is unblocked when all its blockers are closed.
- When closing an issue with an explanation, post the comment first,
  then close the issue.
- Tracker configuration does not grant permission to publish.
  Follow the session's authorization rules.

## Pull requests as a triage surface

**PRs as a request surface: no.**

# Domain docs

## Layout

Use a single-context layout for Worklane:

- GLOSSARY.md at the repository root for domain vocabulary.
- docs/adr/ for architectural decision records.

Create these lazily when terms or decisions are resolved.
Setup does not create empty glossary files or placeholder ADRs.

## Before exploration or design

- Read GLOSSARY.md when it exists.
- Read relevant decisions under docs/adr/ when present.
- If GLOSSARY-MAP.md is introduced later, follow its context pointers.
- Continue silently when glossary files or ADRs do not exist.
- Consult docs/reference/architecture-and-layout.md and
  docs/architecture.md for architecture context.
- Consult relevant specs and plans under docs/superpowers/.
  Distinguish planned behavior from implemented behavior using code.

## Vocabulary and decisions

Use glossary terms consistently in issues, designs, code and tests.
Record resolved terminology through the domain-modeling skill.

Surface conflicts with existing ADRs explicitly, including the decision
reference and the reason for reconsidering it.

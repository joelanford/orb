# Conventions

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>: <subject>

[optional body]

[optional trailers]
```

### Type prefixes

| Type | When to use |
|---|---|
| `feat` | New feature or capability |
| `fix` | Bug fix |
| `refactor` | Code restructuring with no behavior change |
| `chore` | Maintenance (deps, CI, tooling) |
| `docs` | Documentation only |
| `test` | Adding or fixing tests |
| `perf` | Performance improvement |

### Rules

- **Subject:** imperative, lowercase after prefix, no period (e.g., `feat: add catalog search command`)
- **Body:** optional; explains the "why" when non-obvious
- **Co-author trailer:** include when AI-assisted (e.g., `Co-Authored-By: Claude ...`)
- **Breaking changes:** add `!` after type (e.g., `feat!: change resolve output format`)

### Examples

```
feat: add catalog search command
```

```
fix: quote YAML 1.1 non-string values in escapeYAMLString

Values like `yes`, `no`, `on`, `off` are interpreted as booleans by
YAML 1.1 parsers. Wrap them in quotes to preserve string semantics.
```

```
chore: bump golang.org/x/net to v0.51.0
```

## Pull Requests

### Title

Match the commit subject line (conventional commit format).

### Description

Use the repository's PR template (`.github/pull_request_template.md`).

## Branch Naming

Free-form slug describing the change:

```
catalog-search
fix-yaml-quoting
bump-deps
helm-getter-plugin
```

Keep it short, lowercase, hyphen-separated. No type prefix required.

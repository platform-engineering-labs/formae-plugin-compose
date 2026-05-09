# Contributing

This document covers local development for plugin authors. For user-facing
plugin docs (configuration, supported resources, examples), see
[README.md](README.md).

## Prerequisites

- Go 1.25+
- [Pkl CLI](https://pkl-lang.org/main/current/pkl-cli/index.html)
- Docker with Compose v2 plugin

## Local Installation

```bash
git clone https://github.com/platform-engineering-labs/formae-plugin-compose.git
cd formae-plugin-compose
make install
```

This builds the plugin binary and installs it to `~/.pel/formae/plugins/compose/`. The formae agent discovers installed plugins automatically on startup.

## Build and Test

```bash
make build           # Build plugin binary
make test            # Run all tests (unit + integration)
make lint            # Run linter
make verify-schema   # Validate PKL schema
make install         # Build + install locally
```

## Local Testing

```bash
make install
formae agent start
formae apply --mode reconcile --watch examples/basic/main.pkl
```

## Conformance Testing

Conformance tests validate the plugin through a full lifecycle: Create, Read/Verify, Update, Replace, Delete, and Discovery.

| Fixture | Purpose |
|---|---|
| `testdata/resource.pkl` | Initial resource creation |
| `testdata/resource-update.pkl` | In-place update (add label to compose file) |
| `testdata/resource-replace.pkl` | Replacement (change projectName, a createOnly field) |

```bash
make conformance-test                  # Latest formae version
make conformance-test VERSION=0.80.1   # Specific version
```

The `scripts/ci/clean-environment.sh` script removes leftover `formae-test-*` compose projects. It runs before and after conformance tests.

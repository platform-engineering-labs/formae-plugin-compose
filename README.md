# Docker Compose Plugin for formae

Manage Docker Compose stacks as infrastructure with [formae](https://docs.formae.io).

This plugin provisions and manages Docker Compose projects by shelling out to the `docker compose` CLI. It supports the full resource lifecycle: create, read, update, delete, and discovery.

## Installation

Requires Go 1.25+, [Pkl CLI](https://pkl-lang.org/main/current/pkl-cli/index.html), and Docker with Compose v2 plugin.

```bash
git clone https://github.com/platform-engineering-labs/formae-plugin-compose.git
cd formae-plugin-compose
make install
```

This builds the plugin binary and installs it to `~/.pel/formae/plugins/compose/`. The formae agent discovers installed plugins automatically on startup.

## Supported Resources

| Resource Type | Description |
|---|---|
| `Docker::Compose::Stack` | A Docker Compose project managed via `docker compose up/down` |

## Target Configuration

The plugin connects to a Docker daemon. The default configuration uses the local Docker socket:

```pkl
new formae.Target {
  label = "local-docker"
  namespace = "DOCKER"
  config = new compose.Config {}
}
```

The `compose.Config` class exposes:

| Field | Default | Description |
|---|---|---|
| `host` | `unix:///var/run/docker.sock` | Docker host URI |

Docker credentials (registry auth, TLS certs) are read from the environment, not from the target config.

## Schema Fields

The `compose.Stack` resource has the following fields:

| Field | Type | Mutable | Description |
|---|---|---|---|
| `projectName` | `String` | No (createOnly) | Compose project name, used as native ID. Changing triggers replacement. |
| `composeFile` | `String` | Yes | Compose YAML content. Changes trigger `docker compose up`. |
| `endpoints` | `Mapping<String, String>?` | Yes | Named endpoint declarations (`name` -> `service:port`). Resolved to actual URLs after apply. |
| `status` | `String?` | Read-only | Current project status from Docker. |

## Resolvable

A `compose.Stack` exposes a `StackResolvable` for cross-plugin references:

```pkl
local myStack: compose.Stack = ...
// Reference from another resource:
myStack.res.endpoints    // resolves to the endpoints mapping
myStack.res.projectName  // resolves to the project name
```

## Examples

### Basic

A single nginx container:

```bash
formae apply --mode reconcile --watch examples/basic/main.pkl
```

See [examples/basic/main.pkl](examples/basic/main.pkl).

### LGTM Observability Stack

Grafana LGTM all-in-one (Loki, Grafana, Tempo, Mimir) with OpenTelemetry collector:

```bash
formae apply --mode reconcile --watch examples/lgtm/main.pkl
```

See [examples/lgtm/main.pkl](examples/lgtm/main.pkl).

## Development

### Prerequisites

- Go 1.25+
- [Pkl CLI](https://pkl-lang.org/main/current/pkl-cli/index.html)
- Docker with Compose v2 plugin

### Build and Test

```bash
make build           # Build plugin binary
make test            # Run all tests (unit + integration)
make lint            # Run linter
make verify-schema   # Validate PKL schema
make install         # Build + install locally
```

### Local Testing

```bash
make install
formae agent start
formae apply --mode reconcile --watch examples/basic/main.pkl
```

### Conformance Testing

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

## Licensing

Plugins are independent works and may be licensed under any license of the author's choosing.

See the formae plugin policy: <https://docs.formae.io/plugin-sdk/>

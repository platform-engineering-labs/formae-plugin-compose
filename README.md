# Docker Compose Plugin for formae

Manage Docker Compose stacks as infrastructure with [formae](https://docs.formae.io).

This plugin provisions and manages Docker Compose projects by shelling out to the `docker compose` CLI. It supports the full resource lifecycle: create, read, update, delete, and discovery.

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

## Licensing

Licensed under FSL-1.1-ALv2. See [LICENSE](LICENSE).

# Changelog

All notable changes to the formae Docker Compose plugin are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Install with `sudo formae plugin install compose` on the host that runs the
formae agent.

## [0.1.2]

### Changed

- **Resource type renamed to `DOCKER::Compose::Stack`**: the namespace prefix is
  now uppercase to match the formae convention (`namespace = "DOCKER"`). The
  schema generates this name automatically, so PKL forma files that import the
  compose schema don't need any change. Forma files that reference the resource
  type by string (e.g. in queries) should switch from `Docker::Compose::Stack`
  to `DOCKER::Compose::Stack`.

## [0.1.1]

### Added

- **Pick individual endpoints by name**: You can now reference a specific
  endpoint from a compose stack instead of getting the entire endpoints map. Use
  `.at("service:port")` on the endpoints resolvable:

    ```pkl
    url = lgtmStack.res.endpoints.at("lgtm:3000")
    ```

    This makes it easy to wire compose stack services into other plugin targets,
    like connecting Grafana to an LGTM stack in a single forma file.

## [0.1.0]

### Added

- Initial release of the Docker Compose plugin as a standalone package built on
  the formae Plugin SDK.

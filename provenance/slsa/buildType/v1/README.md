# LUCI CIPD SLSA BuildType v1 spec

This file documents the format for SLSA v1 `buildType` for CIPD packages. The spec may be extended in the future.

Upstream values do not change regardless of usage.

This spec is written to aid LUCI developers to properly create their
specifications. While the specification is written as JSON, if you can use the
proto files from [the In-ToTo protocol buffer
definitions](https://github.com/in-toto/attestation/tree/2eae9974359a44270fce20fd9405c08867a0c1ff). The
proto files have schemas that can be checked at compile-time (for statically
checked langugages), while the JSON files are embedded text JSON within those
protocol buffers that have no schema.

## Statement

| Field           | Description            | Example                                                                                           |
|-----------------|------------------------|---------------------------------------------------------------------------------------------------|
| `_type`         | Statement type/version | (From [upstream](https://slsa.dev/spec/v1.0/provenance#schema)) <https://in-toto.io/Statement/v1> |
| `subject`       | See Subject            | See Subject                                                                                       |
| `predicateType` | Predicate type/version | (From [upstream](https://slsa.dev/spec/v1.0/provenance#schema)) <https://slsa.dev/provenance/v1>  |
| `predicate`     | See Predicate          | See Predicate                                                                                     |

## Subject

Array containing one object:

| Field         | Description                | Example                                    |
|---------------|----------------------------|--------------------------------------------|
| `name`        | CIPD package name          | `git/linux-amd64`                          |
| `digest.sha1` | CIPD instance ID/SHA1 hash | `7fd1a60b01f91b314f59955a4e4d4e80d8edf11d` |

## Predicate

| Field             | Description          | Example              |
| ----------------- | -------------------- | -------------------- |
| `buildDefinition` | See Build Definition | See Build Definition |
| `runDetails`      | See Run Details      | See Run Details      |

## Build definition

| Field                                             | Description                                               | Example                                                                                               |
|---------------------------------------------------|-----------------------------------------------------------|-------------------------------------------------------------------------------------------------------|
| `buildType`                                       | Link to this document                                     | `https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/main/provenance/slsa/buildType/v1` |
| `resolvedDependencies`                            | See Resolved Dependencies                                 | See Resolved Dependencies                                                                             |
| `externalParameters.entryPointSource.uri`         | Location/version of recipe repository in SPDX v2.3 format | `git+https://fuchsia.googlesource.com/infra/recipes@ref/heads/main`                                   |
| `externalParameters.entryPointSource.recipe_path` | Recipe entry point (relative path from repository root)   | `recipes.py`                                                                                          |

## Resolved dependencies

This should follow the [upstream spec](https://slsa.dev/spec/v1.0/provenance#schema).

This section MUST include the resolved `entryPointSource`, like so:

| Field                 | Description                                               | Example                                                             |
|-----------------------|-----------------------------------------------------------|---------------------------------------------------------------------|
| `name`                | Static value                                              | `entryPointSource`                                                  |
| `uri`                 | Location/version of recipe repository in SPDX v2.3 format | `git+https://fuchsia.googlesource.com/infra/recipes@ref/heads/main` |
| `digest.gitCommit`    | Exact Git commit hash that was resolved for the build     | `7fd1a60b01f91b314f59955a4e4d4e80d8edf11d`                          |
| `annotations.comment` | Static value                                              | `Resolved from entryPointSource CIPD package`                       |

## Run details

| Field        | Description                    | Example                                   |
| ------------ | ------------------------------ | ----------------------------------------- |
| `builder.id` | Chosen ID registered with BCID | `//bcid.corp.google.com/builders/luci/l2` |

# Unified LUCI CLI Tool (`luci`)

This tool provides a unified command-line interface to interact with all LUCI resources (Analysis, ResultDB, etc.), following resource-oriented design principles (AIP-121).

## Development Status

> [!IMPORTANT]
> This tool is currently in early development and is **not** distributed through binfs or CIPD yet. LUCI developers must build and test it locally.

## For Developers

### Building the Tool

To compile the `luci` binary locally:

```bash
cd client/cmd/luci
go build .
```

### Running Tests

To run tests (ensure integration tests environment is configured):

```bash
INTEGRATION_TESTS=1 go test ./...
```

### Architecture & Subcommands

*   `main.go`: Root command setup. Registers authentication and sub-apps from subpackages.
*   `base/`: Shared CLI utilities and authentication flags.
*   `format/`: Common formatting and HTML summary processing helpers.
*   `testresult/`: Subpackage for `test-result` resource management (e.g. `luci test-result get`, `luci test-result artifact ...`).
*   `workunit/`: Subpackage for `work-unit` resource management (e.g. `luci work-unit get`, `luci work-unit artifact ...`).
*   `verdict/`: Subpackage for `verdict` resource management (e.g. `luci verdict get`).
*   `artifact/`: Core package for artifact fetching and streaming (`get`, `head`, `tail`).

Subcommands are nested using the `subcommands.Application` wrapper to enforce nested sub-actions.

## Manual Testing & Authentication

The tool automatically integrates with your local credentials. If running on corp, run `gcert` first to authenticate automatically.

```bash
# Verify authentication
./luci auth-info

# Get a test verdict by UI URL or resource name
./luci verdict get "https://ci.chromium.org/ui/test-investigate/invocations/build-.../modules/.../schemes/.../variants/.../cases/..."

# Get a test result (by URL, full name, or decomposed IDs)
./luci test-result get "invocations/build-8676.../tests/.../results/0"
./luci test-result get build-8676... "ninja://..." 0

# Get full test result artifact content or byte range
./luci test-result artifact get "invocations/build-8676.../tests/.../results/0" snippet
./luci test-result artifact get "invocations/build-8676.../tests/.../results/0" snippet -byte-range 0-500

# Fetch first / last lines of a test result artifact
./luci test-result artifact head "invocations/build-8676.../tests/.../results/0" snippet
./luci test-result artifact tail "invocations/build-8676.../tests/.../results/0" snippet

# Caching and '-' Substitution:
# A leading '-' means "use the last thing", and you can override trailing components:
./luci test-result get inv testid resultid
./luci test-result artifact get - artifactid              # uses last test result, overrides artifactid
./luci test-result artifact get - resultid2 artifactid     # uses last inv and testid, overrides resultid and artifactid
./luci test-result artifact get - testid2 resultid2 art    # uses last inv, overrides testid, resultid, and artifactid
./luci test-result get -                                  # uses last test result
./luci test-result get - resultid2                        # uses last inv and testid, overrides resultid
./luci test-result get - testid2 resultid2                # uses last inv, overrides testid and resultid

# Work unit management and hierarchy invalidations:
./luci work-unit get "rootInvocations/build-8676.../workUnits/run-tests"
./luci work-unit artifact get - stdout                    # uses last work unit, overrides artifactid
# Note: Changing parent resource (e.g. invocation) automatically unsets/invalidates child resources.
```



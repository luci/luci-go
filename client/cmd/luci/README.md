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
*   `ids/`: Subcommand `luci ids <target>` to parse URLs / resource names and extract component IDs.
*   `testresult/`: Subpackage for `test-result` resource management (`luci test-result get`, `luci test-result artifact ...`).
*   `workunit/`: Subpackage for `work-unit` resource management (`luci work-unit get`, `luci work-unit artifact ...`).
*   `verdict/`: Subpackage for `verdict` resource management (`luci verdict get`).
*   `artifact/`: Core package for artifact queries and streaming (`list`, `get`, `head`, `tail`).

Commands are completely stateless and require explicit flags (`-invocationid`, `-testid`, `-resultid`, `-workunitid`, `-artifactid`). Use `luci ids <url>` to extract IDs from URLs or canonical resource names.

## Manual Testing & Authentication

The tool automatically integrates with your local credentials. If running on corp, run `gcert` first to authenticate automatically.

```bash
# Verify authentication
./luci auth-info

# Extract IDs from a URL or resource name
./luci ids "https://ci.chromium.org/ui/test-investigate/invocations/build-.../modules/.../schemes/.../variants/.../cases/..."
./luci ids "https://android-build.corp.google.com/test_investigate/invocation/I.../test/TR..."

# Get a test verdict
./luci verdict get -invocationid build-8676... -testid "ninja://..." [-varianthash ...]

# Get a test result
./luci test-result get -invocationid build-8676... -testid "ninja://..." -resultid 0

# List test result artifacts
./luci test-result artifact list -invocationid build-8676... -testid "ninja://..." -resultid 0

# Get full test result artifact content or byte range
./luci test-result artifact get -invocationid build-8676... -testid "ninja://..." -resultid 0 -artifactid snippet
./luci test-result artifact get -invocationid build-8676... -testid "ninja://..." -resultid 0 -artifactid snippet -byte-range 0-500

# Fetch first / last lines of a test result artifact
./luci test-result artifact head -invocationid build-8676... -testid "ninja://..." -resultid 0 -artifactid snippet
./luci test-result artifact tail -invocationid build-8676... -testid "ninja://..." -resultid 0 -artifactid snippet

# Work unit management
./luci work-unit get -invocationid build-8676... -workunitid run-tests
./luci work-unit artifact list -invocationid build-8676... -workunitid run-tests
./luci work-unit artifact get -invocationid build-8676... -workunitid run-tests -artifactid stdout
```



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
*   `testresult/`: Subpackage for `test-result` resource management (e.g. `luci test-result get`).
*   `verdict/`: Subpackage for `verdict` resource management (e.g. `luci verdict get`).

Subcommands are nested using the `subcommands.Application` wrapper to enforce nested sub-actions.

## Manual Testing & Authentication

The tool automatically integrates with your local credentials. If running on corp, run `gcert` first to authenticate automatically.

```bash
# Verify authentication
./luci auth-info

# Get a test verdict by UI URL or resource name
./luci verdict get "https://ci.chromium.org/ui/test-investigate/invocations/build-.../modules/.../schemes/.../variants/.../cases/..."
```

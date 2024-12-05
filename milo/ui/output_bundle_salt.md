# Output Bundle Salt

This file serves as salt to the output bundle. Updating this file allows you to
trigger a deployment without a code change to the UI.

You can simply update the version number below to trigger a new build.

```
VERSION=7
```

## Why do we need this?
The primary use case is documented [here](./docs/guides/run_updated_e2e_tests_in_luci_ui_promoter.md).

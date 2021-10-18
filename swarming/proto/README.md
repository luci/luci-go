# Swarming protos

Source:

*   Repo: https://chromium.googlesource.com/infra/luci/luci-py
*   Path: appengine/swarming/proto
*   Revision: 2ef3669860f3d174105d269f84cccba741da5040

## Updating

1.  Run `./update.sh`
1.  Update README.md with git commit hash printed.
1.  NOTE: Due to a discrepancy between luci-go and luci-py, you also need to
    revert the `luci.file_metadata` file option from `config/*.proto`.
1.  Run `go generate ./...`

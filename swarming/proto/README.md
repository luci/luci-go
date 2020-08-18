# Swarming protos

Source:

*   Repo: https://chromium.googlesource.com/infra/luci/luci-py
*   Path: appengine/swarming/proto
*   Revision: cf408cb4d1ce3b4736ef39caa1199defac1bae1b

## Updating

1.  Run `./update.sh`
1.  Update README.md with git commit hash printed.
1.  NOTE: Due to a discrepancy between luci-go and luci-py, you also need to
    revert the `luci.file_metadata` file option from `config/*.proto`.
1.  Run `go generate ./...`

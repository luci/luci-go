# Swarming protos

Source:

*   Repo: https://chromium.googlesource.com/infra/luci/luci-py
*   Path: appengine/swarming/proto
*   Revision: 2edc96392884645fa6701a3c1b96ef840f0d7b8c

## Updating

1.  Run `./update.sh`
1.  Update README.md with git commit hash printed.
1.  NOTE: Due to a discrepancy between luci-go and luci-py, you also need to:
  1. Revert the `luci.file_metadata` file option from `config/*.proto`
  1. Re-run `go generate ./config`

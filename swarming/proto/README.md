# Swarming protos

`*.proto` files in subdirectories are copied from luci-py verbatim:

Repo: https://chromium.googlesource.com/infra/luci/luci-py
Path: appengine/swarming/proto
Revision: bae210eeb82d8f960ed2235cd3bf4a4776c2d8dc

## Updating

1. Copy the proto files from luci-py:

        cp ../../../../../../luci/appengine/swarming/proto/api/*.proto api
        cp ../../../../../../luci/appengine/swarming/proto/config/*.proto config

1. Run `go generate ./...`
1. Update README.md

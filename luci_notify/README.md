# LUCI Notify

## Running tests locally

```
INTEGRATION_TESTS=1 go test ./...
```

## Release

Releases for LUCI Notify are handled via LUCI CD. Configuration is in
the infra/data/gae/apps/luci-notify directory.

To cut a release to prod, run:
```
cd ~/your_infra_checkout/data/gae/scripts
git checkout main
git pull
git checkout -b my-luci-notify-release
./promote.py --canary --stable --commit luci-notify
git cl upload
```
and have your CL approved.

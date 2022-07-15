# luci-go: LUCI services and tools in Go

[![Go
Reference](https://pkg.go.dev/badge/go.chromium.org/luci.svg)](https://pkg.go.dev/go.chromium.org/luci)


## Installing

LUCI Go code is meant to be worked on from an Chromium
[infra.git](https://chromium.googlesource.com/infra/infra.git) checkout, which
enforces packages versions and Go toolchain version. First get fetch via
[depot_tools.git](https://chromium.googlesource.com/chromium/tools/depot_tools.git)
then run:

    fetch infra
    cd infra/go
    eval `./env.py`
    cd src/go.chromium.org/luci

It is now possible to directly install tools with go install:

    go install go.chromium.org/luci/auth/client/cmd/...@latest
    go install go.chromium.org/luci/buildbucket/cmd/...@latest
    go install go.chromium.org/luci/cipd/client/cmd/...@latest
    go install go.chromium.org/luci/client/cmd/...@latest
    go install go.chromium.org/luci/cv/cmd/...@latest
    go install go.chromium.org/luci/gce/cmd/...@latest
    go install go.chromium.org/luci/grpc/cmd/...@latest
    go install go.chromium.org/luci/logdog/client/cmd/...@latest
    go install go.chromium.org/luci/luci_notify/cmd/...@latest
    go install go.chromium.org/luci/lucicfg/cmd/...@latest
    go install go.chromium.org/luci/luciexe/legacy/cmd/...@latest
    go install go.chromium.org/luci/mailer/cmd/...@latest
    go install go.chromium.org/luci/mmutex/cmd/...@latest
    go install go.chromium.org/luci/resultdb/cmd/...@latest
    go install go.chromium.org/luci/server/cmd/...@latest
    go install go.chromium.org/luci/swarming/cmd/...@latest
    go install go.chromium.org/luci/tokenserver/cmd/...@latest
    go install go.chromium.org/luci/tools/cmd/...@latest


## Contributing

Contributing uses the same flow as [Chromium
contributions](https://www.chromium.org/developers/contributing-code).

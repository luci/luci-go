# luci-go: LUCI services and tools in Go

[![GoDoc](https://godoc.org/github.com/luci/luci-go?status.svg)](https://godoc.org/github.com/luci/luci-go)
[![Build Status](https://travis-ci.org/luci/luci-go.svg?branch=master)](https://travis-ci.org/luci/luci-go)
[![Coverage Status](https://coveralls.io/repos/luci/luci-go/badge.svg?branch=master&service=github)](https://coveralls.io/github/luci/luci-go?branch=master)


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


## Contributing

Contributing uses the same flow as [Chromium
contributions](https://www.chromium.org/developers/contributing-code).

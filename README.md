# luci-go: LUCI services and tools in Go 

[![GoDoc](https://godoc.org/go.chromium.org/luci?status.svg)](https://godoc.org/go.chromium.org/luci)


## Instaling

LUCI Go code is meant to be worked on from a Chromium
[infra.git](https://chromium.googlesource.com/infra/infra.git) chekout, which
enforces packages versions and Go toolchain version. First get fetch via 
[depot_tools.git](https://chromium.googlesource.com/chromium/tools/depot_tools.git)
then run:

    fetch infra
    cd infra/go
    eval `./env.py`
    cd src/go.chromium.org/luci


## Contribting 

Contributing uses the same flow as [Chromium
contributions](https://www.chromium.org/developers/contributing-code).

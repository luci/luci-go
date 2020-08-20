# gae: A Google AppEngine SDK wrapper

*designed for testing and extensibility*

#### **THIS PACKAGE HAS NO API COMPATIBILITY GUARANTEES. USE UNPINNED AT YOUR OWN PERIL.**
(but generally it should be pretty stableish).

[![GoDoc](https://godoc.org/go.chromium.org/gae?status.svg)](https://godoc.org/go.chromium.org/gae)


## Installing

LUCI Go GAE adaptor code is meant to be worked on from an Chromium
[infra.git](https://chromium.googlesource.com/infra/infra.git) checkout, which
enforces packages versions and Go toolchain version. First get `fetch` via
[depot_tools.git](https://chromium.googlesource.com/chromium/tools/depot_tools.git)
then run:

    fetch infra
    cd infra/go
    eval `./env.py`
    cd src/go.chromium.org/gae


## Contributing

Contributing uses the same flow as [Chromium
contributions](https://www.chromium.org/developers/contributing-code).

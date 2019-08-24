#!/bin/bash

# Note: this file is explicitly *not* used by go:generate because it depends on
# a preconfigured Javascript development environment which is not available on
# all developer machines. See web/README.md for how to set it up.

# Build RPCExplorer JS app.
../../web.py gulp-app rpcexplorer clean
../../web.py build rpcexplorer

# Convert it into a giant internal/assets.gen.go file.
go run go.chromium.org/luci/tools/cmd/assets \
    -dest-pkg internal \
    ../../dist/rpcexplorer

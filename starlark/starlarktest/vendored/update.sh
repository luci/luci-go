#!/bin/bash

#
# Copies assert.star from go.starlark.net module and makes it available to
# LUCI Go code.
#

set -e

# The path to the unpacked go.starlark.net module in the module cache directory.
MODULE_DIR=`go list -f '{{.Dir}}' -m go.starlark.net`

function vendor() {
  mkdir -p `dirname ${1}`
  cp ${MODULE_DIR}/${1} ${1}
  chmod 0666 ${1}
}

vendor LICENSE
vendor starlarktest/assert.star

go run go.chromium.org/luci/tools/cmd/assets -ext "*.star"

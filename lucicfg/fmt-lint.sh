#!/bin/bash

set -e

MYPATH=$(dirname "${BASH_SOURCE[0]}")
LUCICFG="${MYPATH}/.lucicfg"

trap "rm -f ${LUCICFG}" EXIT
go build -o ${LUCICFG} go.chromium.org/luci/lucicfg/cmd/lucicfg

# Format all Starlark code. Skip testsdata/pkg for now, since it contains
# purposefully non-formattable "fmt-rules-compat" directory.
#
# TODO: resume formatting of testsdata/pkg once "fmt-rules-compat" is gone.
${LUCICFG} fmt \
    "${MYPATH}/starlark" \
    "${MYPATH}/examples" \
    "${MYPATH}/testdata/core"

# Lint only stdlib and examples, but not tests, no one cares.
${LUCICFG} lint "${MYPATH}/starlark" "${MYPATH}/examples"

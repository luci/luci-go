#!/bin/bash

set -e

MYPATH=$(dirname "${BASH_SOURCE[0]}")
LUCICFG="${MYPATH}/.lucicfg"

trap "rm -f ${LUCICFG}" EXIT
go build -o ${LUCICFG} go.chromium.org/luci/lucicfg/cmd/lucicfg

# Format all Starlark code.
${LUCICFG} fmt "${MYPATH}"

# Lint only stdlib and examples, but not tests, no one cares.
${LUCICFG} lint "${MYPATH}/starlark" "${MYPATH}/examples"

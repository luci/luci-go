#!/bin/bash

set -e
trap "rm -f .lucicfg" EXIT

go build -o .lucicfg go.chromium.org/luci/lucicfg/cmd/lucicfg
./.lucicfg fmt && ./.lucicfg lint

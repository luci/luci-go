#!/bin/bash

# This is needed for protoc to understand "absolute" imports in *.proto.
cd ../../../../..

DIR=go.chromium.org/luci/starlark/starlarkprotov2/testprotos

protoc \
    --include_imports \
    --include_source_info \
    -o $DIR/all.pb \
    $DIR/*.proto

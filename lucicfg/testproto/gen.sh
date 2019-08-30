#!/bin/bash

# Generate *.pb.go to be consumed by Go unit tests.
cproto

# Generate test_descpb.bin with descriptor set consumed by Starlark unit tests.
cd ../../../../
protoc \
    --descriptor_set_out go.chromium.org/luci/lucicfg/testdata/misc/support/test_descpb.bin \
    go.chromium.org/luci/lucicfg/testproto/test.proto

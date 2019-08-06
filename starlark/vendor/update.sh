#!/bin/bash
set -e

# Get tip of HEAD.
rm -rf google.golang.org
git clone https://go.googlesource.com/protobuf google.golang.org/protobuf
cd google.golang.org/protobuf

# Record its revision for posterity.
git rev-parse HEAD > ../../REVISION

# Apply local patches.
git apply --check ../../patches/*.patch
git am ../../patches/*.patch

# Trim unnecessary fat, leaving essentially only non-test *.go and legal stuff.
rm -rf benchmarks
rm -rf cmd
rm -rf compiler
rm -rf encoding/testprotos
rm -rf internal/cmd
rm -rf internal/testprotos
rm -rf prototest
rm -rf testing
rm .gitignore .travis.yml go.mod go.sum *.md

find . -type f -name '*_test.go' -delete
find . -type f -name '*.bash' -delete

rm -rf .git

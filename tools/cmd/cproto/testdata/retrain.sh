#!/bin/sh
# Copyright 2016 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

cd ${0%/*}
go install ..

for dir in */; do
    pushd $dir
    cproto
    for go in *.go; do
      golden=${go%*.go}.golden
      mv $go $golden
      gofmt -w -s $golden
    done
    popd
done

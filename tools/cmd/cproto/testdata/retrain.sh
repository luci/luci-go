#!/bin/sh
# Copyright 2016 The Chromium Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

cd ${0%/*}
go install ..

for dir in */; do
    pushd $dir
    cproto
    for go in *.go; do
      mv $go ${go%*.go}.golden
    done
    popd
done

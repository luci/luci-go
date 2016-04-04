#!/bin/sh
#
# Copyright 2016 The Chromium Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
#
# Script for .travis.yaml to install protobuf compiler from source. This is
# needed because APT package for protobuf doesn't support proto3.

set -ex

V="2.6.1"

wget https://github.com/google/protobuf/releases/download/v${V}/protobuf-${V}.tar.gz
tar -xzvf protobuf-${V}.tar.gz
cd protobuf-${V} && ./configure --prefix=/usr && make && sudo make install

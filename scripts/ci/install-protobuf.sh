#!/bin/sh
#
# Copyright 2016 The Chromium Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
#
# Script for .travis.yaml to install protobuf compiler from source. This is
# needed because APT package for protobuf doesn't support proto3.

set -ex

V="3.0.0-beta-2"

wget https://github.com/google/protobuf/archive/v${V}.tar.gz
tar -xzvf v${V}.tar.gz
cd protobuf-${V} && ./autogen.sh && ./configure --prefix=/usr && make -j2 && sudo make install

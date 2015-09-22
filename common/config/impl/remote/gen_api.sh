#!/bin/bash
# Copyright 2015 The Chromium Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

google-api-go-generator -discoveryurl="https://luci-config.appspot.com/_ah/api/discovery/v1/apis" -cache=false -gendir=. -api="config:v1"

rm -rf generated_api
mv config generated_api
mv api-list.json generated_api

# Add the license to the generated code.
GEN_FILE="generated_api/v1/config-gen.go"
head -4 remote.go > tmp
mv $GEN_FILE tmp.go
mv tmp $GEN_FILE
cat tmp.go >> $GEN_FILE
rm tmp.go

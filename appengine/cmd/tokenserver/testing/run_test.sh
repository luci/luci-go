#!/bin/bash
# Copyright 2016 The Chromium Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# run_test.sh runs the actual test.
#
# It assumes devserver.sh and crlserver.sh run in background already.

cd $(dirname $0)
. ./include.sh

go install -v github.com/luci/luci-go/client/cmd/rpc

# Make a CA, feed its config to the token server.
echo "Initializing CA..."
initialize_ca
import_config
fetch_crl

# TODO: write more when there's a client

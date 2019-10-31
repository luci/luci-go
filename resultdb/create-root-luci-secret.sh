#!/bin/bash
# Copyright 2019 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

# This script writes "luci-root-secret" k8s secret.
# The script exists in case we need to recreate a cluster.
# TODO(nodir): remove it in favor of a generated Make file.
set -e

# Generate a random string used to produce various LUCI tokens. It can be
# arbitrary, but must be shared by all processes in the cluster (thus it is
# pregenerated once and stored as k8s secret).
RANDOM_SECRET_B64=`cat /dev/urandom | head -c 32 | base64`
kubectl create secret generic root-luci-secret \
    --from-literal="secret.json={\"current\": \"$RANDOM_SECRET_B64\"}" \

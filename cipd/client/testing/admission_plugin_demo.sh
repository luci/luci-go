#!/bin/bash
# Copyright 2020 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

set -e

CIPD_DEV_BACKEND="https://chrome-infra-packages-dev.appspot.com"


# Staging area for all produced files.
rm -rf .tmp_dir
mkdir .tmp_dir
STAGING=$PWD/.tmp_dir


# `banner title` prints a banner to make logs more exciting.
function banner() {
  echo "======================================================================="
  echo "$1"
  echo "======================================================================="
}


# `calc_sha256 file` calculates SHA256 of a file.
function calc_sha256() {
  if hash sha256sum 2> /dev/null ; then
    sha256sum "$1" | cut -d' ' -f1
  elif hash shasum 2> /dev/null ; then
    shasum -a 256 "$1" | cut -d' ' -f1
  else
    return 1
  fi
}


# `build_fake_package title path` builds a "unique" package file locally.
function build_fake_package {
  banner "Building $2"

  # Generate inputs for a fake package.
  rm -rf "$STAGING/inputs"
  mkdir "$STAGING/inputs"
  echo "$1:" `date` > "$STAGING/inputs/date"

  # Build the package file locally without uploading it anywhere.
  cipd pkg-build \
    -in "$STAGING/inputs" \
    -name playground/testing \
    -out "$2"
}


# Use binaries from this repo.
banner "Building go code"
mkdir $STAGING/bin
go build -v -o "$STAGING/bin/cipd" go.chromium.org/luci/cipd/client/cmd/cipd
go build -v -o "$STAGING/bin/example" go.chromium.org/luci/cipd/client/cipd/plugin/plugins/admission/example
export PATH="$STAGING/bin":$PATH


# Build 3 different instances of the package.
build_fake_package "a" "$STAGING/a.cipd"
build_fake_package "b" "$STAGING/b.cipd"
build_fake_package "c" "$STAGING/c.cipd"


# Upload the first one attaching the "valid" metadata accepted by the example
# admission plugin. It looks for "allowed-sha256" metadata keys with values
# matching package's SHA256. We calculate SHA256 from scratch here, but it is
# also possible to use -json-output from `cipd pkg-build` step to get it.
banner "Registering a.cipd"
cipd pkg-register "$STAGING/a.cipd" \
  -service-url "$CIPD_DEV_BACKEND" \
  -hash-algo sha256 \
  -ref latest-a \
  -metadata allowed-sha256:`calc_sha256 "$STAGING/a.cipd"` \
  -metadata allowed-sha256:also-some-bogus-one-why-not


# Upload the second one, but attach "invalid" metadata which will be rejected
# by the example admission plugin. We reuse metadata intended for `a` here for
# this purpose: it is not valid for `b`.
banner "Registering b.cipd"
cipd pkg-register "$STAGING/b.cipd" \
  -service-url "$CIPD_DEV_BACKEND" \
  -hash-algo sha256 \
  -ref latest-b \
  -metadata allowed-sha256:`calc_sha256 "$STAGING/a.cipd"` \
  -metadata allowed-sha256:also-some-bogus-one-why-not


# Upload the third one, but attach no metadata at all. The plugin should reject
# it as well.
banner "Registering c.cipd"
cipd pkg-register "$STAGING/c.cipd" \
  -service-url "$CIPD_DEV_BACKEND" \
  -hash-algo sha256 \
  -ref latest-c


# Disable any active plugins (if any) for the following check.
export CIPD_ADMISSION_PLUGIN=
export CIPD_CONFIG_FILE="-"

# Verify all packages can be installed when *not* using admission checks.
banner "Installing without admission checks, should succeed"
cipd ensure -ensure-file - -root "$STAGING/0" -service-url "$CIPD_DEV_BACKEND" <<EOF
@Subdir a
playground/testing latest-a
@Subdir b
playground/testing latest-b
@Subdir c
playground/testing latest-c
EOF


# Configure the CIPD client to use the example admission plugin built above.
# The value of CIPD_ADMISSION_PLUGIN env var should be a JSON-serialized list of
# strings with the full command line of the plugin.
export CIPD_ADMISSION_PLUGIN="[\"$STAGING/bin/example\"]"


# Package `a` by itself can be installed, it has valid metadata.
banner "Installing a valid package with admission checks, should succeed"
cipd ensure -ensure-file - -root "$STAGING/1" -service-url "$CIPD_DEV_BACKEND" <<EOF
@Subdir a
playground/testing latest-a
EOF


# This now should fail saying that two instances (`b` and `c`) are not allowed.
banner "Installing invalid packages with admission checks, should fail"
cipd ensure -ensure-file - -root "$STAGING/2" -service-url "$CIPD_DEV_BACKEND" <<EOF
@Subdir a
playground/testing latest-a
@Subdir b
playground/testing latest-b
@Subdir c
playground/testing latest-c
EOF

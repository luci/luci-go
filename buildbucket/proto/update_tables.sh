#!/bin/sh

set -e

# This little script is just to remember the incantation to update the bigquery
# schema. If you don't know what this is, you don't need to run it (and likely
# don't have permission to anyhow).

# Fist, make sure you run up-to-date bqschemaupdater,
# since it's not installed by default in Infra's Go Env.
go install go.chromium.org/luci/tools/cmd/bqschemaupdater

# Actually update BQ schemas, -dev before prod.
bqschemaupdater -message buildbucket.v2.Build -table cr-buildbucket-dev.raw.completed_builds
bqschemaupdater -message buildbucket.v2.Build -table cr-buildbucket.raw.completed_builds

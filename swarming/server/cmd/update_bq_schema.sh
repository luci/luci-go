#!/bin/sh
# Copyright 2024 The LUCI Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

THIS_DIR=$(dirname "$0")

read -p "Cloud Project name to push BQ schema to (default to \"chromium-swarm-dev\"): " BQ_PROJECT
if [ -z "${BQ_PROJECT}" ]; then
  BQ_PROJECT=chromium-swarm-dev
fi

read -p "BigQuery Dataset to push BQ schema to (default to \"swarming\"): " BQ_DATASET
if [ -z "${BQ_DATASET}" ]; then
  BQ_DATASET=swarming
fi

go install go.chromium.org/luci/tools/cmd/bqschemaupdater

# Note: all expirations are 1.5 years.

bqschemaupdater \
  -table "${BQ_PROJECT}.${BQ_DATASET}.task_requests" \
  -message-dir "${THIS_DIR}/../../proto/api" \
  -message "swarming.v1.TaskRequest" \
  -partitioning-field "create_time" \
  -partitioning-expiration 12960h

bqschemaupdater \
  -table "${BQ_PROJECT}.${BQ_DATASET}.bot_events" \
  -message-dir "${THIS_DIR}/../../proto/api" \
  -message "swarming.v1.BotEvent" \
  -partitioning-field "event_time" \
  -partitioning-expiration 12960h

bqschemaupdater \
  -table "${BQ_PROJECT}.${BQ_DATASET}.task_results_run" \
  -message-dir "${THIS_DIR}/../../proto/api" \
  -message "swarming.v1.TaskResult" \
  -partitioning-field "end_time" \
  -partitioning-expiration 12960h

bqschemaupdater \
  -table "${BQ_PROJECT}.${BQ_DATASET}.task_results_summary" \
  -message-dir "${THIS_DIR}/../../proto/api" \
  -message "swarming.v1.TaskResult" \
  -partitioning-field "end_time" \
  -partitioning-expiration 12960h

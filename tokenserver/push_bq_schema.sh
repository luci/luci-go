#!/bin/bash
# Copyright 2018 The LUCI Authors.
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

THIS_DIR=$(dirname "$0")

read -p "Cloud Project name to push BQ schema to: " PROJECT_ID

bqschemaupdater \
  -table "$PROJECT_ID.tokens.delegation_tokens" \
  -friendly-name "Issued delegation tokens." \
  -message-dir "$THIS_DIR/api/bq" \
  -message "tokenserver.bq.DelegationToken" \
  -partitioning-expiration "8760h"  # 1y

bqschemaupdater \
  -table "$PROJECT_ID.tokens.machine_tokens" \
  -friendly-name "Issued machine tokens." \
  -message-dir "$THIS_DIR/api/bq" \
  -message "tokenserver.bq.MachineToken" \
  -partitioning-expiration "2160h"  # 90d

bqschemaupdater \
  -table "$PROJECT_ID.tokens.oauth_token_grants" \
  -friendly-name "Issued OAuth token grants." \
  -message-dir "$THIS_DIR/api/bq" \
  -message "tokenserver.bq.OAuthTokenGrant" \
  -partitioning-expiration "8760h"  # 1y

bqschemaupdater \
  -table "$PROJECT_ID.tokens.oauth_tokens" \
  -friendly-name "Issued OAuth tokens." \
  -message-dir "$THIS_DIR/api/bq" \
  -message "tokenserver.bq.OAuthToken" \
  -partitioning-expiration "8760h"  # 1y

bqschemaupdater \
  -table "$PROJECT_ID.serviceaccounts.project_scoped" \
  -friendly-name "Project scoped service accounts." \
  -message-dir "$THIS_DIR/api/bq" \
  -message "tokenserver.bq.ProjectScopedServiceAccount" \
  -partiioning-expiration "8760h"   # 1y

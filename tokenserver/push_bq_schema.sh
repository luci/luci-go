#!/bin/bash

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
  -table "$PROJECT_ID.tokens.project_tokens" \
  -friendly-name "Issued project tokens." \
  -message-dir "$THIS_DIR/api/bq" \
  -message "tokenserver.bq.ProjectToken" \
  -partitioning-expiration "8760h"  # 1y

bqschemaupdater \
  -table "$PROJECT_ID.tokens.service_account_tokens" \
  -friendly-name "Issued service account tokens." \
  -message-dir "$THIS_DIR/api/bq" \
  -message "tokenserver.bq.ServiceAccountToken" \
  -partitioning-expiration "8760h"  # 1y

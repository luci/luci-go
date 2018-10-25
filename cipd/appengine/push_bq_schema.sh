#!/bin/bash

THIS_DIR=$(dirname "$0")

read -p "Cloud Project name to push BQ schema to: " PROJECT_ID

bqschemaupdater \
  -table "$PROJECT_ID.cipd.events" \
  -friendly-name "CIPD event log." \
  -message-dir "$THIS_DIR/../api/cipd/v1" \
  -message "cipd.Event" \
  -partitioning-field "when"

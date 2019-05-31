#!/bin/bash

THIS_DIR=$(dirname "$0")

read -p "Cloud Project name to push BQ schema to: " PROJECT_ID

bqschemaupdater \
  -table "$PROJECT_ID.cipd.events" \
  -friendly-name "CIPD event log." \
  -message-dir "$THIS_DIR/../api/cipd/v1" \
  -message "cipd.Event" \
  -partitioning-field "when"

# Note: this table is used as a template table to create 'exported_tags_<jobid>'
# tables that contain tags exported by a single specific mapper job. Each such
# individual table is an approximate "snapshot" of the state of tags at the time
# the job ran.
bqschemaupdater \
  -table "$PROJECT_ID.cipd.exported_tags" \
  -friendly-name "Tags exported via EXPORT_TAGS_TO_BQ mapper job." \
  -message-dir "$THIS_DIR/../api/cipd/v1" \
  -message "cipd.ExportedTag" \
  -disable-partitioning  # doesn't work with template tables

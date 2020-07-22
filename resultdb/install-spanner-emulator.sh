#!/usr/bin/env bash

sudo apt-get install google-cloud-sdk-spanner-emulator
gcloud config configurations create spanner-emulator
gcloud config set auth/disable_credentials true
gcloud config set project chops-spanner-testing
gcloud config set api_endpoint_overrides/spanner http://localhost:9020/

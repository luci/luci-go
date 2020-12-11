#!/bin/bash

go run go.chromium.org/luci/client/cmd/swarming trigger \
  -server https://chromium-swarm.appspot.com \
  -service-account chromium-ci-builder@chops-service-accounts.iam.gserviceaccount.com \
  -dimension pool=luci.chromium.ci \
  -dimension machine_type=e2-standard-8 \
  -dimension os=Ubuntu-16.04.6 \
  -dimension zone=us-central1-b \
  -priority 20 \
  -task-name "Linux upload isolated tests: ci/linux-bfcache-rel/11981" \
  -digest 43829ebb66c32b4cedc2e33eada5cad034d79c753a8399d8d4cbf86871083f22/350 \
  --raw-cmd -- python run.py --isolateserve=isolateserver.appspot.com --isolated_tests=isolated_tests.json --cas_instance=chromium-swarm

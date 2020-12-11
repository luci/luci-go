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
  -digest 45bc1baf9b59469e55108608af14b44ca028babc199be25d173457c7a802a784/430 \
  --raw-cmd -- python run.py --isolateserve=isolateserver.appspot.com --isolated_tests=isolated_tests.json --cas_instance=chromium-swarm

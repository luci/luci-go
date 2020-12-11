#!/bin/bash

go run go.chromium.org/luci/client/cmd/swarming trigger \
  -server https://chromium-swarm.appspot.com \
  -service-account chromium-try-builder@chops-service-accounts.iam.gserviceaccount.com \
  -dimension pool=luci.chromium.try \
  -dimension builder=linux-rel \
  -dimension machine_type=n1-standard-8 \
  -dimension os=Ubuntu-16.04 \
  -dimension zone=us-central1-b \
  -priority 20 \
  -task-name "Linux upload isolated tests: linux-rel/566322" \
  -digest e3e82b60a2a490a7344c09765b8e658dc14a0e7a27ffb2d1afac97a948a28ef8/430 \
  --raw-cmd -- python run.py --isolateserve=isolateserver.appspot.com --isolated_tests=isolated_tests.json --cas_instance=chromium-swarm

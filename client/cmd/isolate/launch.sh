#!/bin/bash

# go run go.chromium.org/luci/client/cmd/swarming trigger \
#   -server https://chromium-swarm.appspot.com \
#   -service-account chromium-try-builder@chops-service-accounts.iam.gserviceaccount.com \
#   -dimension pool=luci.chromium.try \
#   -dimension builder=linux-rel \
#   -dimension machine_type=n1-standard-8 \
#   -dimension os=Ubuntu-16.04 \
#   -dimension zone=us-central1-b \
#   -priority 20 \
#   -task-name "Linux upload isolated tests: try/linux-rel/566322" \
#   -digest cc40dc3b2b779a57a0c18850d85e49fea16d9e0802e427cdaf5ddf74dd6c2364/1265 \
#   --raw-cmd -- python run.py --isolateserve=isolateserver.appspot.com --isolated_tests=try_linux-rel_566322.json --cas_instance=chromium-swarm

go run go.chromium.org/luci/client/cmd/swarming trigger \
  -server https://chromium-swarm.appspot.com \
  -service-account chromium-try-builder@chops-service-accounts.iam.gserviceaccount.com \
  -dimension pool=luci.chromium.try \
  -dimension machine_type=n1-highcpu-8 \
  -dimension os=Mac-10.15 \
  -dimension ssd=1 \
  -dimension zone=us-atl-golo-m4 \
  -priority 20 \
  -task-name "Mac upload isolated tests: try/mac-rel/554551" \
  -digest b2abf290e77de2c800788566dc0cc227649e8ee1f9d388c7bf20fa856f9c0920/798 \
  --raw-cmd -- python run.py --isolateserve=isolateserver.appspot.com --isolated_tests=try_mac-rel_554551.json --cas_instance=chromium-swarm

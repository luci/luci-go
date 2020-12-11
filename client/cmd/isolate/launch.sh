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
#   -digest e3e82b60a2a490a7344c09765b8e658dc14a0e7a27ffb2d1afac97a948a28ef8/430 \
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
  -digest 0b32af470cc5090e318ca5c2fb8231eb3e82cf17db468c29a404c82c3cae51b1/1265 \
  --raw-cmd -- python run.py --isolateserve=isolateserver.appspot.com --isolated_tests=try_mac-rel_554551.json --cas_instance=chromium-swarm

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
#   --raw-cmd -- python3 run.py --isolateserve=isolateserver.appspot.com --isolated_tests=try_linux-rel_566322.json --cas_instance=chromium-swarm

# go run go.chromium.org/luci/client/cmd/swarming trigger \
#   -server https://chromium-swarm.appspot.com \
#   -service-account chromium-try-builder@chops-service-accounts.iam.gserviceaccount.com \
#   -dimension pool=luci.chromium.try \
#   -dimension machine_type=n1-highcpu-8 \
#   -dimension os=Mac-10.15 \
#   -dimension ssd=1 \
#   -dimension zone=us-atl-golo-m4 \
#   -priority 20 \
#   -task-name "Mac upload isolated tests: try/mac-rel/554551" \
#   -digest fc66ec80c9d36abf932cd4fba4a103514d2eb43295b3b6c971c8fcf149702fd5/1188 \
#  --raw-cmd -- python run.py --isolateserve=isolateserver.appspot.com --isolated_tests=try_mac-rel_554551.json --cas_instance=chromium-swarm

go run go.chromium.org/luci/client/cmd/swarming trigger \
  -server https://chromium-swarm.appspot.com \
  -service-account chromium-try-builder@chops-service-accounts.iam.gserviceaccount.com \
  -env-prefix VPYTHON_VIRTUALENV_ROOT=cache/vpython \
  -dimension cores=8 \
  -dimension pool=luci.chromium.try \
  -dimension os=Windows-10 \
  -dimension ssd=1 \
  -priority 20 \
  -task-name "Windows upload isolated tests: try/win10_chromium_x64_rel_ng/739255" \
  -digest 2283f3a0369054e680d5b65bace8697d80bab0571a8b56e36bb376062986bb90/1304 \
  --raw-cmd -- C:\\setup\\depot_tools\\vpython run.py --isolateserve=isolateserver.appspot.com --isolated_tests=try_win10_chromium_x64_rel_ng_739422.json --cas_instance=chromium-swarm

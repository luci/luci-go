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
  -digest 1be38be7fe5c8bd6d4ef615d25a0d9e0b7a068b58c23b38af3d117d32783ce31/914 \
  --raw-cmd -- python run.py --isolateserve=isolateserver.appspot.com --isolated_tests=try_mac-rel_554551.json --cas_instance=chromium-swarm

# go run go.chromium.org/luci/client/cmd/swarming trigger \
#   -server https://chromium-swarm.appspot.com \
#   -service-account chromium-tester@chops-service-accounts.iam.gserviceaccount.com \
#   -dimension cores=8 \
#   -dimension pool=chromium.tests \
#   -dimension os=Windows-10 \
#   -priority 20 \
#   -task-name "Windows upload isolated tests: try/win10_chromium_x64_rel_ng/739255" \
#   -digest 1be38be7fe5c8bd6d4ef615d25a0d9e0b7a068b58c23b38af3d117d32783ce31/914 \
#   --raw-cmd -- python run.py --isolateserve=isolateserver.appspot.com --isolated_tests=try_win10_chromium_x64_rel_ng_739255.json --cas_instance=chromium-swarm
# 
  # -dimension ssd=1 \
  # -dimension zone=us-central1-f \
  # -dimension machine_type=n1-standard-8 \
  # chromium-try-@chops-service-accounts.iam.gserviceaccount.com \
  # -env-prefix PATH=cipd_bin_packages:cipd_bin_packages/bin:cipd_bin_packages/cpython:cipd_bin_packages/cpython/bin:cipd_bin_packages/cpython3:cipd_bin_packages/cpython3/bin \
  # -env-prefix VPYTHON_VIRTUALENV_ROOT=cache/vpython \

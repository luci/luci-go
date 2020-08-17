#!/bin/sh
set -eux

# time go run go.chromium.org/luci/client/cmd/cas archive -token-server-host luci-token-server-dev.appspot.com \
#    -cas-instance chromium-swarm-dev -paths  ~/chromium/src/third_party/blink/web_tests/fast: -digest-json json.json

# jq -r '.hash + "/" + .sizeBytes' json.json

# exit

GOOS=windows go build go.chromium.org/luci/client/cmd/cas

go run go.chromium.org/luci/client/cmd/isolated archive \
   -isolate-server 'https://isolateserver-dev.appspot.com' \
   -files '.:cas.exe' -files '.:benchmark.py' -isolated if

hash=$(sha1sum if | awk '{print $1}')
# https://chrome-infra-packages.appspot.com/p/infra/tools/luci/cas/linux-amd64/+/

go run go.chromium.org/luci/client/cmd/swarming trigger \
   -server 'https://chromium-swarm-dev.appspot.com' \
   -dimension pool=chromium.tests -dimension os=Windows-10 \
   -priority 20 \
   -service-account chromium-tester-dev@chops-service-accounts.iam.gserviceaccount.com \
   -isolate-server 'https://isolateserver-dev.appspot.com' \
   -isolated "$hash" \
   -raw-cmd -- python ./benchmark.py

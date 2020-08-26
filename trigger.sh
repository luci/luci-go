#!/bin/sh
set -eux

# time go run go.chromium.org/luci/client/cmd/cas archive -token-server-host luci-token-server-dev.appspot.com \
#    -cas-instance chromium-swarm-dev -paths  ~/chromium/src/third_party/blink/web_tests/fast: -digest-json json.json

# time go run go.chromium.org/luci/client/cmd/isolate archive -I isolateserver-dev.appspot.com \
#      -isolate isolate.isolate -dump-json isolated.json
# exit
# time go run go.chromium.org/luci/client/cmd/isolated archive -I isolateserver-dev.appspot.com \
#      -dirs  ~/chromium/src/third_party/blink/web_tests: -dump-hash hash

# jq -r '.hash + "/" + .sizeBytes' json.json

# exit

# isolated web_tests hash
# 4d26c4f4f66a051e3e931d74830053194eea07b9

# isolated download -I isolateserver-dev.appspot.com -isolated \
#    4d26c4f4f66a051e3e931d74830053194eea07b9 -output-dir hoge



GOOS=linux go build -o tmp/cas.exe go.chromium.org/luci/client/cmd/cas
GOOS=linux go build -o tmp/isolated.exe go.chromium.org/luci/client/cmd/isolated

ISOLATED=tmp/isolated
go run go.chromium.org/luci/client/cmd/isolated archive \
   -isolate-server 'https://isolateserver-dev.appspot.com' \
   -files 'tmp:isolated.exe' -files 'tmp:cas.exe' -files '.:benchmark.py' -isolated $ISOLATED

hash=$(sha1sum $ISOLATED | awk '{print $1}')
# https://chrome-infra-packages.appspot.com/p/infra/tools/luci/cas/linux-amd64/+/

go run go.chromium.org/luci/client/cmd/swarming trigger \
   -server 'https://chromium-swarm-dev.appspot.com' \
   -dimension pool=chromium.tests -dimension os=Ubuntu-16.04 -dimension gce=1 \
   -dimension cores=8 \
   -priority 20 \
   -service-account chromium-tester-dev@chops-service-accounts.iam.gserviceaccount.com \
   -isolate-server 'https://isolateserver-dev.appspot.com' \
   -isolated "$hash" \
   -raw-cmd -- \
   python ./benchmark.py

   # isolated download -I isolateserver-dev.appspot.com -isolated \
   #     4d26c4f4f66a051e3e931d74830053194eea07b9 -output-dir hoge -cache-dir cache -verbose

#!/bin/sh
set -eux

GOOS=windows go test -c go.chromium.org/luci/common/data/caching/cache

go run go.chromium.org/luci/client/cmd/isolated archive \
   -isolate-server 'https://isolateserver-dev.appspot.com' \
   -files '.:cache.test.exe' -isolated if

hash=$(sha1sum if | awk '{print $1}')


go run go.chromium.org/luci/client/cmd/swarming trigger \
   -server 'https://chromium-swarm-dev.appspot.com' \
   -dimension pool=chromium.tests -dimension os=Windows-10 \
   -priority 20 \
   -service-account chromium-tester-dev@chops-service-accounts.iam.gserviceaccount.com \
   -isolate-server 'https://isolateserver-dev.appspot.com' \
   -isolated "$hash" \
   -raw-cmd -- ./cache.test.exe -test.cpu 1 -test.bench .

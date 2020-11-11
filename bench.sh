#!/bin/sh
set -eux

os=windows
# os=linux
# os=darwin

GOOS=$os go test -o tmp/bench -c ./common/data/caching/cache

function os2d() {
    case $1 in
        "windows" ) echo -dimension os=Windows -dimension gce=1 ;;
        "linux" ) echo -dimension os=Ubuntu-16.04 -dimension gce=1 ;;
        "darwin" ) echo -dimension os=Mac ;;
    esac
}

go run go.chromium.org/luci/client/cmd/isolated archive \
   -isolate-server 'https://isolateserver.appspot.com' \
   -files 'tmp:bench' -isolated tmp/if

hash=$(sha1sum tmp/if | awk '{print $1}')

go run go.chromium.org/luci/client/cmd/swarming trigger \
   -server 'https://chromium-swarm.appspot.com' \
   -dimension pool=chromium.tests  $(os2d $os)\
   -dimension ssd=0 \
   -dimension cores=8 -priority 10 \
   -tag debug:1 \
   -isolated "${hash}" \
   -raw-cmd -- ./bench -test.run XXX -test.bench . -test.cpu 1

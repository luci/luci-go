#!/bin/sh
set -eux


os=windows
# os=linux
# os=darwin

function os2d() {
    case $1 in
        "windows" ) echo -dimension os=Windows -dimension gce=1 ;;
        "linux" ) echo -dimension os=Ubuntu-16.04 -dimension gce=1 ;;
        "darwin" ) echo -dimension os=Mac ;;
    esac
}

GOOS=$os go build -o tmp/cas.exe ./client/cmd/cas
# GOOS=$os go build -o tmp/isolated.exe ./client/cmd/isolated

go run go.chromium.org/luci/client/cmd/isolated archive \
   -isolate-server 'https://isolateserver.appspot.com' \
   -files 'tmp:cas.exe' -isolated tmp/if

hash=$(sha1sum tmp/if | awk '{print $1}')

# use the same tree with https://chromium-swarm.appspot.com/task?id=4f6c814ca6627010
go run go.chromium.org/luci/client/cmd/swarming trigger \
   -server 'https://chromium-swarm.appspot.com' \
   -dimension pool=chromium.tests  $(os2d "${os}") \
   -dimension cores=8 -priority 20 \
   -service-account chromium-tester@chops-service-accounts.iam.gserviceaccount.com \
   -tag debug:1 \
   -isolated "${hash}" \
   -raw-cmd -- ./cas.exe download -cas-instance chromium-swarm -digest  16ebd9ef18e96d80d5c450e49460c97e68c3b9dbb4ba1790b3774f506478a915/405 -cache-dir cache -dir out -log-level info

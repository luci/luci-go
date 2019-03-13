#!/bin/bash

luci-auth context -scopes https://www.googleapis.com/auth/devstorage.read_write bash -c 'cp $LUCI_CONTEXT /tmp/luci_context && sleep 1000000' &
LUCI_AUTH_PID=$!

docker run \
        -e INFRADOCK_NO_UPDATE=1  \
        -e LUCI_CONTEXT=/tmp/luci_context \
        -v /tmp/luci_context:/tmp/luci_context \
        --network host \
        gcr.io/chromium-container-registry/infra_dev_env \
        apack \
        pack \
        $@

kill $LUCI_AUTH_PID

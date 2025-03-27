#!/bin/bash -ex
cd `dirname -- $0`
go run go.chromium.org/luci/common/git/testrepo/manage_patchrepo "$@"

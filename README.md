luci-go: LUCI in Go
===================

[![GoDoc](https://godoc.org/github.com/luci/luci-go?status.svg)](https://godoc.org/github.com/luci/luci-go)
[![Build Status](https://travis-ci.org/luci/luci-go.svg?branch=master)](https://travis-ci.org/luci/luci-go)
[![Coverage Status](https://img.shields.io/coveralls/luci/luci-go.svg)](https://coveralls.io/r/luci/luci-go?branch=master)


Installing
----------

    go get -u github.com/luci/luci-go/client/cmd/...
    go get -u github.com/luci/luci-go/server/cmd/...


Code layout
-----------

  * [/appengine/...](https://github.com/luci/luci-go/tree/master/appengine)
    contains [AppEngine](https://cloud.google.com/appengine/docs/go/) server
    code. It imports packages from `/common/...` and `/server/...`.
  * [/client/...](https://github.com/luci/luci-go/tree/master/client) contains
    all client code.
  * [/common/...](https://github.com/luci/luci-go/tree/master/common) contains
    code and structures shared between all of `/appengine/...`, `/client/...`
    and `/server/...`; for example, the structures used by the server APIs.
    These are inherently APIs.
  * [/server/...](https://github.com/luci/luci-go/tree/master/server) contains
    standalone server code. Its packages are reusable by `/appengine/...`.


Versioning
----------

  * Branch `go1` contains the stable code.
  * Branch `master` constains the latest code.


Contributing
------------

  * Sign the [Google CLA](https://cla.developers.google.com/clas).
  * Make sure your `user.email` and `user.name` are configured in `git config`.
  * Install test-only packages:
    `go get -u -t github.com/luci/luci-go/client/...`
  * Install the pre-commit git hook:
    `go get -u github.com/maruel/pre-commit-go && pre-commit-go`

Run the following to setup the code review tool and create your first review:

    git clone https://chromium.googlesource.com/chromium/tools/depot_tools.git $HOME/src/depot_tools
    export PATH="$PATH:$HOME/src/depot_tools"
    cd $GOROOT/github.com/luci/luci-go
    git checkout -b work origin/master

    # hack hack

    git commit -a -m "This is awesome\nR=joe@example.com"
    # This will ask for your Google Account credentials.
    git cl upload -s
    # Wait for LGTM over email.
    git cl land

Use `git cl help` and `git cl help <cmd>` for more details.

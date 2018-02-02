luci-go: LUCI services and tools in Go
======================================

[![GoDoc](https://godoc.org/github.com/luci/luci-go?status.svg)](https://godoc.org/github.com/luci/luci-go)
[![Build Status](https://travis-ci.org/luci/luci-go.svg?branch=master)](https://travis-ci.org/luci/luci-go)
[![Coverage Status](https://coveralls.io/repos/luci/luci-go/badge.svg?branch=master&service=github)](https://coveralls.io/github/luci/luci-go?branch=master)

Installing
----------

    go get -u go.chromium.org/luci/client/cmd/...
    go get -u go.chromium.org/luci/server/cmd/...


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
  * [/deploytool/...](https://github.com/luci/luci-go/tree/master/deploytool)
    contains the LUCI cloud services deployment tool.
  * [/logdog/...](https://github.com/luci/luci-go/tree/master/logdog) contains
    LogDog client and server code, as well as APIs, protobufs, and support
    libraries.
  * [/server/...](https://github.com/luci/luci-go/tree/master/server) contains
    standalone server code. Its packages are reusable by `/appengine/...`.
  * [/tools/...](https://github.com/luci/luci-go/tree/master/tools) contains
    support tools used by other LUCI components.


Versioning
----------

  * Branch `go1` contains the stable code.
  * Branch `master` constains the latest code.


Contributing
------------

  * Sign the [Google CLA](https://cla.developers.google.com/clas).
  * Make sure your `user.email` and `user.name` are configured in `git config`.
  * Install test-only packages:
    `go get -u -t go.chromium.org/luci/client/...`
  * Install the [pcg](https://github.com/maruel/pre-commit-go) git hook:
    `go get -u github.com/maruel/pre-commit-go/cmd/... && pcg`

Run the following to setup the code review tool and create your first review:

    git clone https://chromium.googlesource.com/chromium/tools/depot_tools.git $HOME/src/depot_tools
    export PATH="$PATH:$HOME/src/depot_tools"
    cd $GOROOT/go.chromium.org/luci
    git checkout -b work origin/master

    # hack hack
    git commit -a -m "This is awesome"

    # We use Gerrit for code review. Visit
    # https://chromium-review.googlesource.com/new-password
    # and follow instructions.

    git cl upload -s --r-owners
    # This will upload your change to Gerrit and pick a random owner (as defined
    # in the OWNERS file) for review.
    # Wait for approval and submit your code through Commit Queue.
    # Commit queue will test your change on multiple platforms and land it
    # automatically.

    # Once you get a review with comments, you can do additional commits to your
    # feature branch, and then upload again to update the review in gerrit.
    $ git cl upload

Use `git cl help` and `git cl help <cmd>` for more details.

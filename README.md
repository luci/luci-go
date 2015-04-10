luci-go: LUCI in Go
===================

[![GoDoc](https://godoc.org/github.com/luci/luci-go?status.svg)](https://godoc.org/github.com/luci/luci-go)
[![Build Status](https://travis-ci.org/luci/luci-go.svg?branch=master)](https://travis-ci.org/luci/luci-go)
[![Coverage Status](https://img.shields.io/coveralls/luci/luci-go.svg)](https://coveralls.io/r/luci/luci-go?branch=master)


Installing clients
------------------

    go get -u github.com/luci/luci-go/client/cmd/...


Code layout
-----------

  * `/client/cmd/...` contains executables.
  * `/client/internal/...` contains non API shared internal packages for use by
    other packages in this repository. This includes third parties. See
    https://golang.org/s/go14internal for more details.
  * `/client/...` not in any other of the category is an API package usable
    externally.


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

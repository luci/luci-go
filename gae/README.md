gae: A Google AppEngine SDK wrapper
===================
*designed for testing and extensibility*

#### **THIS PACKAGE HAS NO API COMPATIBILITY GUARANTEES. USE UNPINNED AT YOUR OWN PERIL.**
(but generally it should be pretty stableish).

[![GoDoc](https://godoc.org/github.com/luci/gae?status.svg)](https://godoc.org/github.com/luci/gae)
[![Build Status](https://travis-ci.org/luci/gae.svg?branch=master)](https://travis-ci.org/luci/gae)
[![Coverage Status](https://coveralls.io/repos/luci/gae/badge.svg?branch=master&service=github)](https://coveralls.io/github/luci/gae?branch=master)

Installing
----------

    go get -u go.chromium.org/gae/...

Why/What/How
------------

See the [godocs](https://godoc.org/github.com/luci/gae).

Versioning
----------

  * Branch `master` contains the latest code.

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
